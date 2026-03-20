// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package client

import (
	"context"
	"net"
	"sync"

	DnsParser "masterdnsvpn-go/internal/dnsparser"
)

type localDNSRequest struct {
	buffer []byte
	size   int
	addr   *net.UDPAddr
}

func (c *Client) RunLocalDNSListener(ctx context.Context) error {
	if c == nil || !c.cfg.LocalDNSEnabled {
		return nil
	}

	if err := c.startStream0Runtime(ctx); err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.ParseIP(c.cfg.LocalDNSIP),
		Port: c.cfg.LocalDNSPort,
	})

	if err != nil {
		return err
	}

	defer conn.Close()

	c.log.Infof(
		"\U0001F4E1 <green>Local DNS Listener Ready Addr: <cyan>%s:%d</cyan> (Workers: <cyan>%d</cyan>)</green>",
		c.cfg.LocalDNSIP,
		c.cfg.LocalDNSPort,
		c.cfg.LocalDNSWorkers,
	)

	queue := make(chan localDNSRequest, c.cfg.LocalDNSQueueSize)
	packetPool := sync.Pool{
		New: func() any {
			return make([]byte, EDnsSafeUDPSize)
		},
	}

	var workerWG sync.WaitGroup
	for range c.cfg.LocalDNSWorkers {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			c.localDNSWorker(ctx, conn, queue, &packetPool)
		}()
	}

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	for {
		buffer := packetPool.Get().([]byte)
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			packetPool.Put(buffer)
			if ctx.Err() != nil {
				break
			}
			return err
		}

		select {
		case queue <- localDNSRequest{buffer: buffer, size: n, addr: addr}:
		case <-ctx.Done():
			packetPool.Put(buffer)
			close(queue)
			workerWG.Wait()
			return nil
		default:
			response, _ := DnsParser.BuildServerFailureResponse(buffer[:n])
			if len(response) != 0 {
				_, _ = conn.WriteToUDP(response, addr)
			}
			packetPool.Put(buffer)
		}
	}

	close(queue)
	workerWG.Wait()
	return nil
}

func (c *Client) startStream0Runtime(ctx context.Context) error {
	if c == nil || c.stream0Runtime == nil {
		return nil
	}
	c.ensureLocalDNSCachePersistence(ctx)
	if err := c.stream0Runtime.Start(ctx); err != nil {
		return err
	}
	c.startResolverHealthRuntime(ctx)
	return nil
}

func (c *Client) localDNSWorker(ctx context.Context, conn *net.UDPConn, queue <-chan localDNSRequest, packetPool *sync.Pool) {
	for {
		select {
		case <-ctx.Done():
			return
		case req, ok := <-queue:
			if !ok {
				return
			}
			c.processLocalDNSRequest(conn, req, packetPool)
		}
	}
}

func (c *Client) processLocalDNSRequest(conn *net.UDPConn, req localDNSRequest, packetPool *sync.Pool) {
	defer func() {
		if packetPool != nil && req.buffer != nil {
			packetPool.Put(req.buffer)
		}
		if recovered := recover(); recovered != nil && c != nil && c.log != nil {
			c.log.Errorf(
				"💥 <red>Local DNS Handler Panic: <cyan>%v</cyan></red>",
				recovered,
			)
		}
	}()

	now := c.now()
	response := c.resolveDNSQueryPacket(req.buffer[:req.size], now)
	if len(response) != 0 {
		_, _ = conn.WriteToUDP(response, req.addr)
	}
}

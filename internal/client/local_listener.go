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
	"time"
)

func (c *Client) runLocalTCPAcceptLoop(ctx context.Context, addr string, readyLog func(), handler func(net.Conn)) error {
	if c == nil {
		return nil
	}
	if err := c.startStream0Runtime(ctx); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	if readyLog != nil {
		readyLog()
	}

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go handler(conn)
	}
}

func withLocalConnLifecycle(conn net.Conn, onPanic func(any), fn func() bool) {
	if conn == nil {
		return
	}
	handedOff := false
	defer func() {
		if recovered := recover(); recovered != nil && onPanic != nil {
			onPanic(recovered)
		}
		if !handedOff {
			_ = conn.Close()
		}
	}()
	handedOff = fn()
}

func localHandshakeTimeout(timeout time.Duration, fallback time.Duration) time.Duration {
	if timeout <= 0 {
		return fallback
	}
	return timeout
}

func attachLocalStreamConn(c *Client, streamID uint16, conn net.Conn, timeout time.Duration) {
	if c == nil || conn == nil {
		return
	}
	_ = conn.SetDeadline(time.Time{})
	stream := c.createStream(streamID, conn)
	go c.runLocalStreamReadLoop(stream, timeout)
}

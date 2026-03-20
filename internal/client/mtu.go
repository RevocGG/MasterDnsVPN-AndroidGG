// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package client

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	DnsParser "masterdnsvpn-go/internal/dnsparser"
	Enums "masterdnsvpn-go/internal/enums"
	"masterdnsvpn-go/internal/logger"
	VpnProto "masterdnsvpn-go/internal/vpnproto"
)

var ErrNoValidConnections = errors.New("no valid connections after mtu testing")

const (
	mtuProbeCodeLength  = 4
	mtuProbeRawResponse = 0
	mtuProbeBase64Reply = 1
	EDnsSafeUDPSize     = 4096
	defaultMTUMinFloor  = 30
	defaultUploadMaxCap = 512
)

var (
	maxUploadProbePacketType = VpnProto.MaxHeaderPacketType()
	mtuDownResponseReserve   = func() int {
		reserve := VpnProto.MaxHeaderRawSize() - VpnProto.HeaderRawSize(Enums.PACKET_MTU_DOWN_RES)
		if reserve < 0 {
			return 0
		}
		return reserve
	}()
)

type mtuRejectReason uint8

const (
	mtuRejectNone mtuRejectReason = iota
	mtuRejectUpload
	mtuRejectDownload
)

type mtuProbeOptions struct {
	IsRetry bool
	Quiet   bool
}

type mtuConnectionProbeResult struct {
	UploadBytes   int
	UploadChars   int
	DownloadBytes int
}

type mtuScanCounters struct {
	completed      atomic.Int32
	valid          atomic.Int32
	rejectUpload   atomic.Int32
	rejectDownload atomic.Int32
}

func (c *Client) RunInitialMTUTests(ctx context.Context) error {
	if len(c.connections) == 0 {
		return ErrNoValidConnections
	}

	uploadCaps := c.precomputeUploadCaps()
	workerCount := min(max(1, c.cfg.MTUTestParallelism), len(c.connections))
	c.logMTUStart(workerCount)
	c.prepareMTUSuccessOutputFile()
	for idx := range c.connections {
		c.prepareConnectionMTUScanState(&c.connections[idx])
	}

	counters := &mtuScanCounters{}
	if workerCount <= 1 {
		for idx := range c.connections {
			if err := ctx.Err(); err != nil {
				return nil
			}
			conn := &c.connections[idx]
			c.runConnectionMTUTest(ctx, conn, idx+1, len(c.connections), uploadCaps[conn.Domain], counters)
		}
	} else {
		jobs := make(chan int, len(c.connections))
		var wg sync.WaitGroup
		for i := 0; i < workerCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for idx := range jobs {
					if err := ctx.Err(); err != nil {
						return
					}
					conn := &c.connections[idx]
					c.runConnectionMTUTest(ctx, conn, idx+1, len(c.connections), uploadCaps[conn.Domain], counters)
				}
			}()
		}
		for idx := range c.connections {
			select {
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				return nil
			case jobs <- idx:
			}
		}
		close(jobs)
		wg.Wait()
	}

	c.balancer.RefreshValidConnections()
	validConns, minUpload, minDownload, minUploadChars := summarizeValidMTUConnections(c.connections)
	if len(validConns) == 0 {
		if c.log != nil {
			c.log.Errorf("<red>No valid connections found after MTU testing!</red>")
		}
		return ErrNoValidConnections
	}

	c.applySyncedMTUState(minUpload, minDownload, minUploadChars)
	c.initResolverRecheckMeta()
	c.appendMTUUsageSeparatorOnce()
	c.logMTUCompletion(validConns)
	return nil
}

func (c *Client) prepareConnectionMTUScanState(conn *Connection) {
	if conn == nil {
		return
	}
	conn.IsValid = true
	conn.UploadMTUBytes = 0
	conn.UploadMTUChars = 0
	conn.DownloadMTUBytes = 0
}

func (c *Client) runConnectionMTUTest(ctx context.Context, conn *Connection, serverID int, total int, maxUploadPayload int, counters *mtuScanCounters) {
	if conn == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			conn.IsValid = false
			if c.log != nil {
				c.log.Errorf(
					"💥 <red>MTU Probe Worker Panic: <cyan>%v</cyan> (Resolver: <cyan>%s</cyan>)</red>",
					recovered,
					conn.ResolverLabel,
				)
			}
			if counters != nil {
				completed := counters.completed.Add(1)
				rejectedNow := counters.rejectUpload.Add(1) + counters.rejectDownload.Load()
				if c.log != nil && c.log.Enabled(logger.LevelWarn) {
					c.log.Warnf(
						"<red>❌ Rejected (%d/%d): <cyan>%s</cyan> via <cyan>%s</cyan> | reason=<yellow>PANIC</yellow> | totals: valid=<green>%d</green>, rejected=<red>%d</red></red>",
						completed,
						total,
						conn.Domain,
						conn.ResolverLabel,
						counters.valid.Load(),
						rejectedNow,
					)
				}
			}
		}
	}()

	if c.log != nil && c.log.Enabled(logger.LevelDebug) {
		c.log.Debugf(
			"<green>Testing Resolver: <cyan>%s</cyan> for Domain: <cyan>%s</cyan> (<cyan>%d / %d</cyan>)</green>",
			conn.ResolverLabel,
			conn.Domain,
			serverID,
			total,
		)
	}

	result, reason := c.probeConnectionMTU(ctx, conn, maxUploadPayload)
	if counters == nil {
		return
	}

	switch reason {
	case mtuRejectUpload:
		completed := counters.completed.Add(1)
		rejectedNow := counters.rejectUpload.Add(1) + counters.rejectDownload.Load()
		if c.log != nil && c.log.Enabled(logger.LevelWarn) {
			c.log.Warnf(
				"<red>❌ Rejected (%d/%d): <cyan>%s</cyan> via <cyan>%s</cyan> | reason=<yellow>UPLOAD_MTU</yellow> | value=<cyan>%d</cyan> | totals: valid=<green>%d</green>, rejected=<red>%d</red></red>",
				completed,
				total,
				conn.Domain,
				conn.ResolverLabel,
				result.UploadBytes,
				counters.valid.Load(),
				rejectedNow,
			)
		}
		return
	case mtuRejectDownload:
		completed := counters.completed.Add(1)
		rejectedNow := counters.rejectUpload.Load() + counters.rejectDownload.Add(1)
		if c.log != nil && c.log.Enabled(logger.LevelWarn) {
			c.log.Warnf(
				"<red>❌ Rejected (%d/%d): <cyan>%s</cyan> via <cyan>%s</cyan> | reason=<yellow>DOWNLOAD_MTU</yellow> | value=<cyan>%d</cyan> | totals: valid=<green>%d</green>, rejected=<red>%d</red></red>",
				completed,
				total,
				conn.Domain,
				conn.ResolverLabel,
				result.DownloadBytes,
				counters.valid.Load(),
				rejectedNow,
			)
		}
		return
	}

	conn.IsValid = true
	conn.UploadMTUBytes = result.UploadBytes
	conn.UploadMTUChars = result.UploadChars
	conn.DownloadMTUBytes = result.DownloadBytes

	completed := counters.completed.Add(1)
	validNow := counters.valid.Add(1)
	rejectedNow := counters.rejectUpload.Load() + counters.rejectDownload.Load()
	if c.log != nil && c.log.Enabled(logger.LevelInfo) {
		c.log.Infof(
			"<green>✅ Accepted (%d/%d): <cyan>%s</cyan> via <cyan>%s</cyan> | upload=<cyan>%d</cyan> | download=<cyan>%d</cyan> | totals: valid=<green>%d</green>, rejected=<red>%d</red></green>",
			completed,
			total,
			conn.Domain,
			conn.ResolverLabel,
			conn.UploadMTUBytes,
			conn.DownloadMTUBytes,
			validNow,
			rejectedNow,
		)
	}
	c.appendMTUSuccessLine(conn)
}

func (c *Client) probeConnectionMTU(ctx context.Context, conn *Connection, maxUploadPayload int) (mtuConnectionProbeResult, mtuRejectReason) {
	var result mtuConnectionProbeResult

	probeTransport, err := newUDPQueryTransport(conn.ResolverLabel)
	if err != nil {
		conn.IsValid = false
		return result, mtuRejectUpload
	}
	defer probeTransport.conn.Close()

	upOK, upBytes, upChars, err := c.testUploadMTU(ctx, conn, probeTransport, maxUploadPayload)
	if err != nil || !upOK {
		conn.IsValid = false
		result.UploadBytes = upBytes
		result.UploadChars = upChars
		return result, mtuRejectUpload
	}
	result.UploadBytes = upBytes
	result.UploadChars = upChars

	downOK, downBytes, err := c.testDownloadMTU(ctx, conn, probeTransport, upBytes)
	if err != nil || !downOK {
		conn.IsValid = false
		result.DownloadBytes = downBytes
		return result, mtuRejectDownload
	}
	result.DownloadBytes = downBytes
	return result, mtuRejectNone
}

func (c *Client) precomputeUploadCaps() map[string]int {
	caps := make(map[string]int, len(c.cfg.Domains))
	for _, domain := range c.cfg.Domains {
		if _, exists := caps[domain]; exists {
			continue
		}
		caps[domain] = c.maxUploadMTUPayload(domain)
	}
	return caps
}

func (c *Client) testUploadMTU(ctx context.Context, conn *Connection, probeTransport *udpQueryTransport, maxPayload int) (bool, int, int, error) {
	if maxPayload <= 0 {
		return false, 0, 0, nil
	}
	if c.log != nil && c.log.Enabled(logger.LevelDebug) {
		c.log.Debugf("<cyan>[MTU]</cyan> Testing upload MTU for %s", conn.Domain)
	}

	maxLimit := c.cfg.MaxUploadMTU
	if maxLimit <= 0 || maxLimit > defaultUploadMaxCap {
		maxLimit = defaultUploadMaxCap
	}
	if maxPayload > maxLimit {
		maxPayload = maxLimit
	}

	best := c.binarySearchMTU(
		ctx,
		"upload mtu",
		c.cfg.MinUploadMTU,
		maxPayload,
		func(candidate int, isRetry bool) (bool, error) {
			return c.sendUploadMTUProbe(ctx, conn, probeTransport, candidate, mtuProbeOptions{
				IsRetry: isRetry,
			})
		},
	)
	if best < max(defaultMTUMinFloor, c.cfg.MinUploadMTU) {
		return false, 0, 0, nil
	}
	return true, best, c.encodedCharsForPayload(best), nil
}

func (c *Client) testDownloadMTU(ctx context.Context, conn *Connection, probeTransport *udpQueryTransport, uploadMTU int) (bool, int, error) {
	if c.log != nil && c.log.Enabled(logger.LevelDebug) {
		c.log.Debugf("<cyan>[MTU]</cyan> Testing download MTU for %s", conn.Domain)
	}
	best := c.binarySearchMTU(
		ctx,
		"download mtu",
		c.cfg.MinDownloadMTU,
		c.cfg.MaxDownloadMTU,
		func(candidate int, isRetry bool) (bool, error) {
			return c.sendDownloadMTUProbe(ctx, conn, probeTransport, candidate, uploadMTU, mtuProbeOptions{
				IsRetry: isRetry,
			})
		},
	)
	if best < max(defaultMTUMinFloor, c.cfg.MinDownloadMTU) {
		return false, 0, nil
	}
	return true, best, nil
}

func (c *Client) binarySearchMTU(ctx context.Context, label string, minValue, maxValue int, testFn func(int, bool) (bool, error)) int {
	if maxValue <= 0 {
		return 0
	}

	low := max(minValue, defaultMTUMinFloor)
	high := maxValue
	if high < low {
		if c.log != nil && c.log.Enabled(logger.LevelDebug) {
			c.log.Debugf(
				"<cyan>[MTU]</cyan> Invalid %s range: low=%d, high=%d. Skipping.",
				label,
				low,
				high,
			)
		}
		return 0
	}
	if c.log != nil && c.log.Enabled(logger.LevelDebug) {
		c.log.Debugf(
			"<cyan>[MTU]</cyan> Starting binary search for %s. Range: %d-%d",
			label,
			low,
			high,
		)
	}

	check := func(value int) bool {
		ok := false
		for attempt := 0; attempt < c.mtuTestRetries; attempt++ {
			if err := ctx.Err(); err != nil {
				return false
			}
			passed, err := testFn(value, attempt > 0)
			if err != nil && c.log != nil && c.log.Enabled(logger.LevelDebug) {
				c.log.Debugf("MTU test callable raised for %d: %v", value, err)
			}
			if err == nil && passed {
				ok = true
				break
			}
		}
		return ok
	}

	if check(high) {
		if c.log != nil && c.log.Enabled(logger.LevelDebug) {
			c.log.Debugf("<cyan>[MTU]</cyan> Max MTU %d is valid.", high)
		}
		return high
	}
	if low == high {
		if c.log != nil && c.log.Enabled(logger.LevelDebug) {
			c.log.Debugf(
				"<cyan>[MTU]</cyan> Only one MTU candidate (%d) existed and it failed.",
				low,
			)
		}
		return 0
	}
	if !check(low) {
		if c.log != nil && c.log.Enabled(logger.LevelDebug) {
			c.log.Debugf(
				"<cyan>[MTU]</cyan> Both boundary MTUs failed (min=%d, max=%d). Skipping middle checks.",
				low,
				high,
			)
		}
		return 0
	}

	best := low
	left := low + 1
	right := high - 1
	for left <= right {
		if err := ctx.Err(); err != nil {
			return 0
		}
		mid := (left + right) / 2
		if check(mid) {
			best = mid
			left = mid + 1
		} else {
			right = mid - 1
		}
	}
	if c.log != nil && c.log.Enabled(logger.LevelDebug) {
		c.log.Debugf("<cyan>[MTU]</cyan> Binary search result: %d", best)
	}
	return best
}

func (c *Client) sendUploadMTUProbe(ctx context.Context, conn *Connection, probeTransport *udpQueryTransport, mtuSize int, options mtuProbeOptions) (bool, error) {
	if mtuSize < 1+mtuProbeCodeLength {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	c.logMTUProbe(
		options.IsRetry,
		options.Quiet,
		"<magenta>[MTU Probe]</magenta> Testing Upload MTU: <yellow>%d</yellow> bytes via <cyan>%s</cyan>",
		mtuSize,
		conn.ResolverLabel,
	)

	payload, code, useBase64, err := c.buildMTUProbePayload(mtuSize, 0)
	if err != nil {
		return false, err
	}

	query, err := c.buildMTUProbeQuery(conn.Domain, Enums.PACKET_MTU_UP_REQ, payload)
	if err != nil {
		return false, nil
	}

	response, err := exchangeUDPQuery(probeTransport, query, c.mtuTestTimeout)
	if err != nil {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>⚠️ Upload test failed: Upload MTU <cyan>%d</cyan> bytes via <cyan>%s</cyan> for <cyan>%s</cyan></yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
		return false, nil
	}

	packet, err := DnsParser.ExtractVPNResponse(response, useBase64)
	if err != nil {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>⚠️ Upload test failed: Upload MTU <cyan>%d</cyan> bytes via <cyan>%s</cyan> for <cyan>%s</cyan></yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
		return false, nil
	}
	if !c.validateServerPacket(packet) {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>⚠️ Upload test failed: Upload MTU <cyan>%d</cyan> bytes via <cyan>%s</cyan> for <cyan>%s</cyan></yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
		return false, nil
	}
	if packet.PacketType != Enums.PACKET_MTU_UP_RES {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>⚠️ Upload test failed: Upload MTU <cyan>%d</cyan> bytes via <cyan>%s</cyan> for <cyan>%s</cyan></yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
		return false, nil
	}
	if len(packet.Payload) != 6 {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>⚠️ Upload test failed: Upload MTU <cyan>%d</cyan> bytes via <cyan>%s</cyan> for <cyan>%s</cyan></yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
		return false, nil
	}
	if !bytes.Equal(packet.Payload[:mtuProbeCodeLength], code) {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>⚠️ Upload test failed: Upload MTU <cyan>%d</cyan> bytes via <cyan>%s</cyan> for <cyan>%s</cyan></yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
		return false, nil
	}
	ok := int(binary.BigEndian.Uint16(packet.Payload[mtuProbeCodeLength:mtuProbeCodeLength+2])) == mtuSize
	if ok {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>🟢 Upload test passed: Upload MTU <green>%d</green> bytes via <cyan>%s</cyan> for <cyan>%s</cyan></yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
	} else {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>⚠️ Upload test failed: Upload MTU <cyan>%d</cyan> bytes via <cyan>%s</cyan> for <cyan>%s</cyan></yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
	}
	return ok, nil
}

func (c *Client) sendDownloadMTUProbe(ctx context.Context, conn *Connection, probeTransport *udpQueryTransport, mtuSize int, uploadMTU int, options mtuProbeOptions) (bool, error) {
	if mtuSize < defaultMTUMinFloor {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	c.logMTUProbe(
		options.IsRetry,
		options.Quiet,
		"<magenta>[MTU Probe]</magenta> Testing Download MTU: <yellow>%d</yellow> bytes via <cyan>%s</cyan>",
		mtuSize,
		conn.ResolverLabel,
	)

	effectiveDownloadSize := effectiveDownloadMTUProbeSize(mtuSize)
	if effectiveDownloadSize < defaultMTUMinFloor {
		return false, nil
	}
	requestLen := max(1+mtuProbeCodeLength+2, uploadMTU)
	payload, code, useBase64, err := c.buildMTUProbePayload(requestLen, 2)
	if err != nil {
		return false, err
	}
	binary.BigEndian.PutUint16(payload[1+mtuProbeCodeLength:1+mtuProbeCodeLength+2], uint16(effectiveDownloadSize))

	query, err := c.buildMTUProbeQuery(conn.Domain, Enums.PACKET_MTU_DOWN_REQ, payload)
	if err != nil {
		return false, nil
	}

	response, err := exchangeUDPQuery(probeTransport, query, c.mtuTestTimeout)
	if err != nil {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>⚠️ Download test failed: Download MTU <cyan>%d</cyan> bytes via <cyan>%s</cyan> for <cyan>%s</cyan> (No Response)</yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
		return false, nil
	}

	packet, err := DnsParser.ExtractVPNResponse(response, useBase64)
	if err != nil {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>⚠️ Download test failed: Download MTU <cyan>%d</cyan> bytes via <cyan>%s</cyan> for <cyan>%s</cyan> (Unexpected Packet Type)</yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
		return false, nil
	}
	if !c.validateServerPacket(packet) {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>⚠️ Download test failed: Download MTU <cyan>%d</cyan> bytes via <cyan>%s</cyan> for <cyan>%s</cyan> (Unexpected Packet Type)</yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
		return false, nil
	}
	if packet.PacketType != Enums.PACKET_MTU_DOWN_RES {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>⚠️ Download test failed: Download MTU <cyan>%d</cyan> bytes via <cyan>%s</cyan> for <cyan>%s</cyan> (Unexpected Packet Type)</yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
		return false, nil
	}
	if len(packet.Payload) != effectiveDownloadSize {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>⚠️ Download test failed: Download MTU <cyan>%d</cyan> bytes via <cyan>%s</cyan> for <cyan>%s</cyan> (Data Size Mismatch)</yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
		return false, nil
	}
	if len(packet.Payload) < 1+mtuProbeCodeLength+1 {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>⚠️ Download test failed: Download MTU <cyan>%d</cyan> bytes via <cyan>%s</cyan> for <cyan>%s</cyan> (Data Size Mismatch)</yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
		return false, nil
	}
	if !bytes.Equal(packet.Payload[:mtuProbeCodeLength], code) {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>⚠️ Download test failed: Download MTU <cyan>%d</cyan> bytes via <cyan>%s</cyan> for <cyan>%s</cyan> (Data Size Mismatch)</yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
		return false, nil
	}
	ok := int(binary.BigEndian.Uint16(packet.Payload[mtuProbeCodeLength:mtuProbeCodeLength+2])) == effectiveDownloadSize
	if ok {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>🟢 Download test passed: Download MTU <green>%d</green> bytes via <cyan>%s</cyan> for <cyan>%s</cyan></yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
	} else {
		c.logMTUProbe(
			options.IsRetry,
			options.Quiet,
			"<yellow>⚠️ Download test failed: Download MTU <cyan>%d</cyan> bytes via <cyan>%s</cyan> for <cyan>%s</cyan> (Data Size Mismatch)</yellow>",
			mtuSize,
			conn.ResolverLabel,
			conn.Domain,
		)
	}
	return ok, nil
}

func (c *Client) buildMTUProbeQuery(domain string, packetType uint8, payload []byte) ([]byte, error) {
	return c.buildTunnelTXTQueryRaw(domain, VpnProto.BuildOptions{
		SessionID:      255,
		PacketType:     packetType,
		StreamID:       1,
		SequenceNum:    1,
		FragmentID:     0,
		TotalFragments: 1,
		Payload:        payload,
	})
}

func (c *Client) maxUploadMTUPayload(domain string) int {
	maxChars := DnsParser.CalculateMaxEncodedQNameChars(domain)
	if maxChars <= 0 {
		return 0
	}

	low := 0
	high := maxChars
	best := 0
	for low <= high {
		mid := (low + high) / 2
		if c.canBuildUploadPayload(domain, mid) {
			best = mid
			low = mid + 1
		} else {
			high = mid - 1
		}
	}
	return best
}

func (c *Client) canBuildUploadPayload(domain string, payloadLen int) bool {
	if payloadLen <= 0 {
		return true
	}

	buf := c.udpBufferPool.Get().([]byte)
	defer c.udpBufferPool.Put(buf)

	if payloadLen > len(buf) {
		return false
	}

	payload := buf[:payloadLen]
	// No need to fill data just to check length
	encoded, err := VpnProto.BuildEncoded(VpnProto.BuildOptions{
		SessionID:       255,
		PacketType:      maxUploadProbePacketType,
		SessionCookie:   255,
		StreamID:        0xFFFF,
		SequenceNum:     0xFFFF,
		FragmentID:      0xFF,
		TotalFragments:  0xFF,
		CompressionType: 0xFF,
		Payload:         payload,
	}, c.codec)
	if err != nil {
		return false
	}

	_, err = DnsParser.BuildTunnelQuestionName(domain, encoded)
	return err == nil
}

func (c *Client) buildMTUProbePayload(length int, reservedTailPrefix int) ([]byte, []byte, bool, error) {
	if length <= 0 {
		return nil, nil, false, nil
	}

	payload := make([]byte, length)
	useBase64 := c != nil && c.cfg.BaseEncodeData
	payload[0] = mtuProbeRawResponse
	if useBase64 {
		payload[0] = mtuProbeBase64Reply
	}

	// Use atomic counter for unique probe IDs (Fast & Unique)
	code := make([]byte, mtuProbeCodeLength)
	binary.BigEndian.PutUint32(code, uint32(c.mtuProbeCounter.Add(1)))
	copy(payload[1:1+mtuProbeCodeLength], code)

	fillOffset := 1 + mtuProbeCodeLength + reservedTailPrefix
	if fillOffset < len(payload) {
		fillMTUProbeBytesFast(payload[fillOffset:], uint64(time.Now().UnixNano()))
	}

	return payload, code, useBase64, nil
}

func fillMTUProbeBytesFast(dst []byte, seed uint64) {
	n := len(dst)
	if n == 0 {
		return
	}

	// Fast Xorshift PRNG to fill with non-duplicate data
	state := seed
	if state == 0 {
		state = 0x9e3779b97f4a7c15
	}

	for i := 0; i < n; i += 8 {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		if n-i >= 8 {
			binary.BigEndian.PutUint64(dst[i:i+8], state)
		} else {
			remaining := n - i
			var tmp [8]byte
			binary.BigEndian.PutUint64(tmp[:], state)
			copy(dst[i:], tmp[:remaining])
		}
	}
}


func summarizeValidMTUConnections(connections []Connection) (validConns []Connection, minUpload int, minDownload int, minUploadChars int) {
	validConns = make([]Connection, 0, len(connections))
	for _, conn := range connections {
		if !conn.IsValid {
			continue
		}
		validConns = append(validConns, conn)

		if conn.UploadMTUBytes > 0 && (minUpload == 0 || conn.UploadMTUBytes < minUpload) {
			minUpload = conn.UploadMTUBytes
		}
		if conn.DownloadMTUBytes > 0 && (minDownload == 0 || conn.DownloadMTUBytes < minDownload) {
			minDownload = conn.DownloadMTUBytes
		}
		if conn.UploadMTUChars > 0 && (minUploadChars == 0 || conn.UploadMTUChars < minUploadChars) {
			minUploadChars = conn.UploadMTUChars
		}
	}
	return validConns, minUpload, minDownload, minUploadChars
}

func (c *Client) encodedCharsForPayload(payloadLen int) int {
	if payloadLen <= 0 {
		return 0
	}

	buf := c.udpBufferPool.Get().([]byte)
	defer c.udpBufferPool.Put(buf)

	if payloadLen > len(buf) {
		return 0
	}

	payload := buf[:payloadLen]
	encoded, err := VpnProto.BuildEncoded(VpnProto.BuildOptions{
		SessionID:       255,
		PacketType:      Enums.PACKET_STREAM_DATA,
		SessionCookie:   255,
		StreamID:        0xFFFF,
		SequenceNum:     0xFFFF,
		FragmentID:      0xFF,
		TotalFragments:  0xFF,
		CompressionType: 0xFF,
		Payload:         payload,
	}, c.codec)
	if err != nil {
		return 0
	}
	return len(encoded)
}

func effectiveDownloadMTUProbeSize(downloadMTU int) int {
	if downloadMTU <= 0 {
		return 0
	}
	return downloadMTU + mtuDownResponseReserve
}

func computeSafeUploadMTU(uploadMTU int, cryptoOverhead int) int {
	if uploadMTU <= 0 {
		return 0
	}
	safe := uploadMTU - cryptoOverhead
	if safe < 64 {
		safe = 64
	}
	if safe > uploadMTU {
		return uploadMTU
	}
	return safe
}

func mtuCryptoOverhead(method int) int {
	switch method {
	case 2:
		return 16
	case 3, 4, 5:
		return 28
	default:
		return 0
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

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
	"strconv"
	"time"
)

func (c *Client) RunLocalTCPListener(ctx context.Context) error {
	if c == nil || c.cfg.ProtocolType != "TCP" {
		return nil
	}
	return c.runLocalTCPAcceptLoop(
		ctx,
		net.JoinHostPort(c.cfg.ListenIP, strconv.Itoa(c.cfg.ListenPort)),
		func() {
			c.log.Infof(
				"\U0001F50C <green>Local TCP Listener Ready</green> <magenta>|</magenta> <blue>Addr</blue>: <cyan>%s:%d</cyan>",
				c.cfg.ListenIP,
				c.cfg.ListenPort,
			)
		},
		c.handleLocalTCPConn,
	)
}

func (c *Client) handleLocalTCPConn(conn net.Conn) {
	withLocalConnLifecycle(conn, func(recovered any) {
		if c.log != nil {
			c.log.Errorf(
				"\U0001F4A5 <red>Local TCP Handler Panic Recovered</red> <magenta>|</magenta> <yellow>%v</yellow>",
				recovered,
			)
		}
	}, func() bool {
		timeout := 10 * time.Second
		_ = conn.SetDeadline(time.Now().Add(timeout))

		streamID, err := c.OpenTCPStream(timeout)
		if err != nil {
			return false
		}

		attachLocalStreamConn(c, streamID, conn, timeout)
		return true
	})
}

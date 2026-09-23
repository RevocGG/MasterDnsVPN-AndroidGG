// Package mobile — tun_state.go
//
// Shared state of the tun2socks bridge, kept free of build tags so the
// credentials plumbing can be unit-tested on every platform. The platform
// API lives in tun_api.go (linux||android) and tun_api_stub.go (others).
// ==============================================================================
package mobile

import (
	"context"
	"encoding/binary"
	"net"
	"sync"
)

var (
	tunMu     sync.Mutex
	tunCancel context.CancelFunc
	tunDone   chan struct{}

	// Credentials of the local SOCKS5 proxy (from the profile config).
	// The tun2socks bridge's internal SOCKS5 dialer uses these when the
	// proxy requires username/password authentication. Guarded by tunMu.
	tunSocksAuth bool
	tunSocksUser string
	tunSocksPass string

	// disableIPv6 gates the "Disable IPv6" app toggle. Default true: the DNS
	// tunnel has no IPv6 egress, so AAAA answers (and IPv6 CONNECTs) only
	// produce `socks5 connect refused code 3` timeouts in apps like YouTube.
	// Can be turned off per-profile from the Android settings.
	disableIPv6 = true
)

// SetDisableIPv6 toggles the bridge's IPv6 blocking (app setting "Disable IPv6").
// Safe to call at any time; affects queries/connections from now on.
func SetDisableIPv6(disabled bool) {
	tunMu.Lock()
	disableIPv6 = disabled
	tunMu.Unlock()
}

// GetDisableIPv6 reports whether IPv6 blocking is currently enabled.
func GetDisableIPv6() bool {
	tunMu.Lock()
	defer tunMu.Unlock()
	return disableIPv6
}

// ipv6Blocked reports whether IPv6 traffic must be rejected by the bridge.
func ipv6Blocked() bool {
	tunMu.Lock()
	defer tunMu.Unlock()
	return disableIPv6
}

// currentSocksCred returns the credentials of the local SOCKS5 proxy the
// bridge dials. They are captured by StartTunBridge from the running instance.
func currentSocksCred() socksCred {
	tunMu.Lock()
	defer tunMu.Unlock()
	if !tunSocksAuth || tunSocksUser == "" {
		return socksCred{}
	}
	return socksCred{user: tunSocksUser, pass: tunSocksPass}
}

// socks5CredentialsDialer returns a connection wrapper that injects the
// configured credentials into the SOCKS5 handshake (method 0x02 USER_PASS)
// when authentication is enabled on the local proxy. The returned conn must
// be used for the handshake BEFORE any payload bytes flow. The handshake
// itself lives in socks5_client.go (portable, unit-tested).
func socks5CredentialsDialer(conn net.Conn) net.Conn {
	cred := currentSocksCred()
	if cred.user == "" {
		return conn
	}
	return &socksAuthConn{Conn: conn, cred: cred}
}

// ── DNS AAAA filtering (IPv6 fix for TUN mode) ─────────────────────────────

// dnsHeaderLen is the fixed size of a DNS message header:
// ID(2) Flags(2) QDCOUNT(2) ANCOUNT(2) NSCOUNT(2) ARCOUNT(2).
const dnsHeaderLen = 12

// dnsTypeAAAA is the DNS record type for IPv6 addresses (RFC 3596).
const dnsTypeAAAA = 28

// filterDNSAAAA removes every AAAA record from a DNS response message.
//
// The DNS tunnel can only carry IPv4 connectivity: the remote side answers
// every IPv6 CONNECT with SOCKS5 NETWORK_UNREACHABLE (code=3), which the
// bridge turns into a TCP RST — the app then shows ERR_CONNECTION_RESET.
// Android prefers IPv6 whenever a AAAA answer exists (RFC 6724), so
// advertising AAAA records guarantees broken first attempts for every
// dual-stack site. Rewriting AAAA replies to NOERROR/NODATA makes the
// resolver fall back to the A record immediately, which works end-to-end.
//
// This is the same mitigation used by clash/sing-box tun stacks.
//
// Gated by the app's "Disable IPv6" toggle (default ON). When the toggle is
// off, responses pass through unmodified and the IPv6 TCP fast-RST in the
// bridge is also disabled.
//
// The function returns the original slice untouched when the message is not
// a response carrying AAAA answers, so the common IPv4 path stays zero-copy.
func filterDNSAAAA(resp []byte) []byte {
	if !ipv6Blocked() {
		return resp
	}
	if len(resp) < dnsHeaderLen {
		return resp
	}
	flags := binary.BigEndian.Uint16(resp[2:4])
	if flags&0x8000 == 0 {
		return resp // QR=0: a query, not a response — leave untouched.
	}
	hasRecords := false
	for _, off := range []int{6, 8, 10} { // ANCOUNT, NSCOUNT, ARCOUNT
		if binary.BigEndian.Uint16(resp[off:off+2]) > 0 {
			hasRecords = true
		}
	}
	if !hasRecords {
		return resp
	}

	// Parse the question section to find where the answer records start.
	qEnd, ok := dnsQuestionEnd(resp)
	if !ok {
		return resp
	}

	// Walk the AN+NS+AR sections keeping only non-AAAA records.
	origAN := int(binary.BigEndian.Uint16(resp[6:8]))
	origNS := int(binary.BigEndian.Uint16(resp[8:10]))
	// Records in the wire stream appear in AN, then NS, then AR order.
	// We only need to detect AAAA records inside them; owner-name parsing is
	// unnecessary because every record's fixed part (type/class/ttl/rdlen)
	// follows the variable-length name. Instead of parsing names we rebuild
	// from parsed records: each record's name is resolved by scanning with
	// nameLengthAt which handles compression pointers.
	p := qEnd
	total := origAN + origNS + int(binary.BigEndian.Uint16(resp[10:12]))
	type rec struct {
		start, end int
		isAAAA     bool
	}
	recs := make([]rec, 0, total)
	for i := 0; i < total && p+10 <= len(resp); i++ {
		nameLen, ok := dnsNameLengthAt(resp, p)
		if !ok {
			return resp // malformed — safer to keep the original message.
		}
		rtype := binary.BigEndian.Uint16(resp[p+nameLen : p+nameLen+2])
		rdLen := int(binary.BigEndian.Uint16(resp[p+nameLen+8 : p+nameLen+10]))
		end := p + nameLen + 10 + rdLen
		if end > len(resp) {
			return resp
		}
		recs = append(recs, rec{start: p, end: end, isAAAA: rtype == dnsTypeAAAA})
		p = end
	}
	if p != len(resp) && total > 0 {
		// Trailing bytes we could not attribute to a record — keep original.
		if p > len(resp) {
			return resp
		}
	}

	// Build output: header + question + kept records, then fix the counts.
	kept := 0
	out := make([]byte, 0, len(resp))
	out = append(out, resp[:qEnd]...)
	sectionKept := [3]int{0, 0, 0}
	section := 0
	sectionRemaining := origAN
	for _, r := range recs {
		if sectionRemaining == 0 && section < 2 {
			section++
			switch section {
			case 1:
				sectionRemaining = origNS
			case 2:
				sectionRemaining = int(binary.BigEndian.Uint16(resp[10:12]))
			}
		}
		if sectionRemaining > 0 {
			sectionRemaining--
		}
		if r.isAAAA {
			continue
		}
		sectionKept[section]++
		kept++
		out = append(out, resp[r.start:r.end]...)
	}
	if kept == total {
		return resp // nothing dropped — return the original (zero-copy fast path).
	}
	binary.BigEndian.PutUint16(out[6:8], uint16(sectionKept[0]))
	binary.BigEndian.PutUint16(out[8:10], uint16(sectionKept[1]))
	binary.BigEndian.PutUint16(out[10:12], uint16(sectionKept[2]))
	return out
}

// dnsQuestionEnd returns the offset just past the first question entry
// (QNAME + QTYPE + QCLASS), handling uncompressed and compressed labels.
func dnsQuestionEnd(msg []byte) (int, bool) {
	nameLen, ok := dnsNameLengthAt(msg, dnsHeaderLen)
	if !ok {
		return 0, false
	}
	q := dnsHeaderLen + nameLen + 4 // + QTYPE + QCLASS
	if q > len(msg) {
		return 0, false
	}
	return q, true
}

// dnsNameLengthAt returns the byte length of the (possibly compressed)
// domain name starting at off in a DNS message, not counting any bytes
// reached through compression pointers beyond their 2-byte header.
func dnsNameLengthAt(msg []byte, off int) (int, bool) {
	p := off
	for {
		if p >= len(msg) {
			return 0, false
		}
		l := int(msg[p])
		if l == 0 {
			return p + 1 - off, true
		}
		if l&0xC0 == 0xC0 {
			if p+2 > len(msg) {
				return 0, false
			}
			return p + 2 - off, true
		}
		p += 1 + l
	}
}

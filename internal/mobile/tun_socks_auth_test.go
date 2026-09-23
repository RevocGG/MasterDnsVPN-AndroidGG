package mobile

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

// fakeSocks5Server implements a minimal SOCKS5 server supporting both
// no-auth (0x00) and USER_PASS (0x02) negotiation followed by CONNECT.
// requireAuth selects which method the server will accept.
type fakeSocks5Server struct {
	listener   net.Listener
	requireAuth bool
	authUser   string
	authPass   string

	// gotAuthUser/gotAuthPass record the credentials received (if any).
	gotAuthUser string
	gotAuthPass string

	// lastTarget records the CONNECT target of the last session.
	lastTarget string
}

func newFakeSocks5Server(t *testing.T, requireAuth bool, user, pass string) *fakeSocks5Server {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeSocks5Server{listener: l, requireAuth: requireAuth, authUser: user, authPass: pass}
	go s.serve()
	t.Cleanup(func() { _ = l.Close() })
	return s
}

func (s *fakeSocks5Server) addr() string { return s.listener.Addr().String() }

func (s *fakeSocks5Server) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeSocks5Server) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Greeting: VER NMETHODS METHODS
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return
	}
	methods := make([]byte, int(hdr[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}

	want := byte(0x00)
	if s.requireAuth {
		want = 0x02
	}
	accepted := false
	for _, m := range methods {
		if m == want {
			accepted = true
			break
		}
	}
	if _, err := conn.Write([]byte{0x05, want}); err != nil || !accepted {
		return
	}

	if s.requireAuth {
		ver := make([]byte, 1)
		if _, err := io.ReadFull(conn, ver); err != nil || ver[0] != 0x01 {
			return
		}
		ulen := make([]byte, 1)
		if _, err := io.ReadFull(conn, ulen); err != nil {
			return
		}
		user := make([]byte, int(ulen[0]))
		if _, err := io.ReadFull(conn, user); err != nil {
			return
		}
		plen := make([]byte, 1)
		if _, err := io.ReadFull(conn, plen); err != nil {
			return
		}
		pass := make([]byte, int(plen[0]))
		if _, err := io.ReadFull(conn, pass); err != nil {
			return
		}
		s.gotAuthUser = string(user)
		s.gotAuthPass = string(pass)
		if s.gotAuthUser != s.authUser || s.gotAuthPass != s.authPass {
			_, _ = conn.Write([]byte{0x01, 0x01})
			return
		}
		if _, err := conn.Write([]byte{0x01, 0x00}); err != nil {
			return
		}
	}

	// CONNECT: VER CMD RSV ATYP ...
	req := make([]byte, 4)
	if _, err := io.ReadFull(conn, req); err != nil {
		return
	}
	var host string
	switch req[3] {
	case 0x01:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	case 0x03:
		lb := make([]byte, 1)
		if _, err := io.ReadFull(conn, lb); err != nil {
			return
		}
		dn := make([]byte, int(lb[0]))
		if _, err := io.ReadFull(conn, dn); err != nil {
			return
		}
		host = string(dn)
	case 0x04:
		ip := make([]byte, 16)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	default:
		return
	}
	pb := make([]byte, 2)
	if _, err := io.ReadFull(conn, pb); err != nil {
		return
	}
	s.lastTarget = net.JoinHostPort(host, itoa(int(binary.BigEndian.Uint16(pb))))

	// Success reply with a bound address (IPv4 zero).
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	// Echo loop so the test can verify the data path end-to-end.
	br := bufio.NewReader(conn)
	buf := make([]byte, 1024)
	for {
		n, err := br.Read(buf)
		if n > 0 {
			if _, werr := conn.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}

// connectViaBridge performs the bridge handshake (socks5ConnectTCP wrapped by
// socks5CredentialsDialer) against the fake server and returns the raw conn
// after CONNECT succeeded.
func connectViaBridge(t *testing.T, server *fakeSocks5Server, useAuth bool) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", server.addr())
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	if useAuth {
		// Simulate credentials captured by StartTunBridge.
		tunMu.Lock()
		tunSocksAuth, tunSocksUser, tunSocksPass = true, "alice", "s3cret"
		tunMu.Unlock()
		t.Cleanup(func() {
			tunMu.Lock()
			tunSocksAuth, tunSocksUser, tunSocksPass = false, "", ""
			tunMu.Unlock()
		})
		conn = socks5CredentialsDialer(conn)
	}
	if err := socks5ConnectTCP(conn, "example.com", 443); err != nil {
		_ = conn.Close()
		t.Fatalf("socks5ConnectTCP: %v", err)
	}
	return conn
}

func TestTunBridgeSocks5NoAuthStillWorks(t *testing.T) {
	srv := newFakeSocks5Server(t, false, "", "")
	conn := connectViaBridge(t, srv, false)
	defer conn.Close()
	if srv.gotAuthUser != "" {
		t.Fatalf("server did not expect auth but got user %q", srv.gotAuthUser)
	}
	if srv.lastTarget != "example.com:443" {
		t.Fatalf("unexpected CONNECT target: %q", srv.lastTarget)
	}
}

func TestTunBridgeSocks5WithUserPassAuth(t *testing.T) {
	srv := newFakeSocks5Server(t, true, "alice", "s3cret")
	conn := connectViaBridge(t, srv, true)
	defer conn.Close()

	if srv.gotAuthUser != "alice" || srv.gotAuthPass != "s3cret" {
		t.Fatalf("credentials not forwarded: user=%q pass=%q", srv.gotAuthUser, srv.gotAuthPass)
	}
	if srv.lastTarget != "example.com:443" {
		t.Fatalf("unexpected CONNECT target after auth: %q", srv.lastTarget)
	}

	// Verify the data path: write after handshake must be echoed back.
	payload := []byte("hello-through-tun")
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	buf := make([]byte, len(payload))
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != string(payload) {
		t.Fatalf("echo mismatch: got %q want %q", buf, payload)
	}
}

func TestTunBridgeSocks5AuthWrongPasswordFails(t *testing.T) {
	srv := newFakeSocks5Server(t, true, "alice", "correct")
	// Force mismatching credentials via the wrapper.
	conn, err := net.Dial("tcp", srv.addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	tunMu.Lock()
	tunSocksAuth, tunSocksUser, tunSocksPass = true, "alice", "WRONG"
	tunMu.Unlock()
	t.Cleanup(func() {
		tunMu.Lock()
		tunSocksAuth, tunSocksUser, tunSocksPass = false, "", ""
		tunMu.Unlock()
	})
	wrapped := socks5CredentialsDialer(conn)
	if err := socks5ConnectTCP(wrapped, "example.com", 443); err == nil {
		t.Fatal("expected CONNECT to fail with wrong password, got nil error")
	}
}

package mobile

import (
	"encoding/binary"
	"testing"
)

// buildDNSQuery builds a minimal DNS query for name with the given qtype.
func buildDNSQuery(t *testing.T, name string, qtype uint16) []byte {
	t.Helper()
	msg := []byte{0x12, 0x34, 0x01, 0x00, 0, 1, 0, 0, 0, 0, 0, 0}
	for _, label := range splitDNSName(name) {
		msg = append(msg, byte(len(label)))
		msg = append(msg, label...)
	}
	msg = append(msg, 0x00)
	msg = binary.BigEndian.AppendUint16(msg, qtype)
	msg = binary.BigEndian.AppendUint16(msg, 1) // class IN
	return msg
}

func splitDNSName(name string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(name); i++ {
		if i == len(name) || name[i] == '.' {
			if i > start {
				out = append(out, name[start:i])
			}
			start = i + 1
		}
	}
	return out
}

// appendDNSAnswer appends one answer record (uncompressed owner name) of the
// given type with the provided 4-byte-or-16-byte rdata.
func appendDNSAnswer(msg []byte, name string, rtype uint16, rdata []byte) []byte {
	for _, label := range splitDNSName(name) {
		msg = append(msg, byte(len(label)))
		msg = append(msg, label...)
	}
	msg = append(msg, 0x00)
	msg = binary.BigEndian.AppendUint16(msg, rtype)
	msg = binary.BigEndian.AppendUint16(msg, 1)      // class IN
	msg = binary.BigEndian.AppendUint32(msg, 300)    // TTL
	msg = binary.BigEndian.AppendUint16(msg, uint16(len(rdata)))
	return append(msg, rdata...)
}

func setDNSCounts(msg []byte, an, ns, ar uint16) {
	binary.BigEndian.PutUint16(msg[6:8], an)
	binary.BigEndian.PutUint16(msg[8:10], ns)
	binary.BigEndian.PutUint16(msg[10:12], ar)
}

func TestFilterDNSAAAADropsAAAAKeepsA(t *testing.T) {
	query := buildDNSQuery(t, "example.com", 1) // QTYPE=A? — the reply type is what matters
	resp := append([]byte{}, query...)
	// QR=1, RD=1, RA=1
	resp[2] = 0x81
	resp[3] = 0x80
	v4 := []byte{93, 184, 216, 34}
	v6 := []byte{0x26, 0x06, 0x28, 0x00, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x22, 0x22}
	resp = appendDNSAnswer(resp, "example.com", 1, v4)
	resp = appendDNSAnswer(resp, "example.com", 28, v6)
	resp = appendDNSAnswer(resp, "example.com", 1, []byte{93, 184, 216, 35})
	setDNSCounts(resp, 3, 0, 0)

	got := filterDNSAAAA(resp)
	if len(got) >= len(resp) {
		t.Fatalf("expected filtered message to be shorter: got=%d orig=%d", len(got), len(resp))
	}
	an := binary.BigEndian.Uint16(got[6:8])
	if an != 2 {
		t.Fatalf("expected ANCOUNT=2 after dropping AAAA, got %d", an)
	}
	// Exactly one AAAA record must remain in the wire format.
	aaaas := 0
	qEnd, ok := dnsQuestionEnd(got)
	if !ok {
		t.Fatal("failed to parse question of filtered message")
	}
	p := qEnd
	for p+10 <= len(got) {
		rdLen := int(binary.BigEndian.Uint16(got[p+8 : p+10]))
		typ := binary.BigEndian.Uint16(got[p+2 : p+4])
		if typ == dnsTypeAAAA {
			aaaas++
		}
		p += 10 + rdLen
	}
	if aaaas != 0 {
		t.Fatalf("expected 0 AAAA records after filtering, found %d", aaaas)
	}
	// Header flags must stay intact.
	if got[2] != 0x81 || got[3] != 0x80 {
		t.Fatalf("header flags changed: % x % x", got[2], got[3])
	}
}

func TestFilterDNSAAAAAllAAAAZeroesCounts(t *testing.T) {
	query := buildDNSQuery(t, "v6.example.net", 28)
	resp := append([]byte{}, query...)
	resp[2] = 0x81
	resp[3] = 0x80
	v6 := make([]byte, 16)
	v6[0] = 0x20
	v6[1] = 0x01
	resp = appendDNSAnswer(resp, "v6.example.net", 28, v6)
	resp = appendDNSAnswer(resp, "v6.example.net", 28, v6)
	setDNSCounts(resp, 2, 0, 0)

	got := filterDNSAAAA(resp)
	an := binary.BigEndian.Uint16(got[6:8])
	if an != 0 {
		t.Fatalf("expected ANCOUNT=0 when all answers are AAAA, got %d", an)
	}
	if int(an)+0 != 0 {
		t.Fatal("unreachable")
	}
	// Message must still parse: header + question only.
	qEnd, ok := dnsQuestionEnd(got)
	if !ok {
		t.Fatal("filtered message has no parseable question")
	}
	if len(got) < qEnd {
		t.Fatal("filtered message truncated below question end")
	}
	_ = got
}

func TestFilterDNSAAAALeavesQueriesAndNonAAAAResponses(t *testing.T) {
	// A query (QR=0) must pass through untouched.
	query := buildDNSQuery(t, "example.com", 28)
	if got := filterDNSAAAA(query); &got[0] != &query[0] {
		t.Fatal("expected query to be returned untouched")
	}

	// A response with only A records must pass through untouched.
	resp := buildDNSQuery(t, "example.com", 1)
	resp[2] = 0x81
	resp[3] = 0x80
	resp = appendDNSAnswer(resp, "example.com", 1, []byte{1, 2, 3, 4})
	setDNSCounts(resp, 1, 0, 0)
	if got := filterDNSAAAA(resp); &got[0] != &resp[0] {
		t.Fatal("expected AAAA-free response to be returned untouched")
	}

	// Truncated garbage must not panic and must be returned as-is.
	junk := []byte{0x81, 0x80, 0, 1, 0, 1, 0, 0, 0, 0}
	if got := filterDNSAAAA(junk); len(got) != len(junk) {
		t.Fatal("expected malformed message to be returned unchanged")
	}
}

func TestFilterDNSAAAAAdditionalSection(t *testing.T) {
	query := buildDNSQuery(t, "example.com", 1)
	resp := append([]byte{}, query...)
	resp[2] = 0x81
	resp[3] = 0x80
	resp = appendDNSAnswer(resp, "example.com", 1, []byte{93, 184, 216, 34})
	// Additional-section AAAA (e.g. authoritative server glue).
	resp = appendDNSAnswer(resp, "ns1.example.com", 28, make([]byte, 16))
	setDNSCounts(resp, 1, 0, 1)

	got := filterDNSAAAA(resp)
	ar := binary.BigEndian.Uint16(got[10:12])
	if ar != 0 {
		t.Fatalf("expected ARCOUNT=0 after dropping additional-section AAAA, got %d", ar)
	}
	an := binary.BigEndian.Uint16(got[6:8])
	if an != 1 {
		t.Fatalf("expected ANCOUNT=1 to survive, got %d", an)
	}
}

// TestFilterDNSAAAAToggle verifies the "Disable IPv6" toggle gates the AAAA
// filter: default ON strips AAAA, OFF passes responses through untouched.
func TestFilterDNSAAAAToggle(t *testing.T) {
	restore := GetDisableIPv6()
	defer func() { SetDisableIPv6(restore) }()

	if !GetDisableIPv6() {
		t.Fatalf("Disable IPv6 must default to ON")
	}

	query := buildDNSQuery(t, "ipv6.google.com", 28)
	resp := append([]byte{}, query...)
	resp[2] = 0x81
	resp[3] = 0x80
	v6 := make([]byte, 16)
	v6[0] = 0x20
	v6[1] = 0x01
	resp = appendDNSAnswer(resp, "ipv6.google.com", 1, []byte{142, 250, 0, 100}) // A
	resp = appendDNSAnswer(resp, "ipv6.google.com", 28, v6)                      // AAAA
	setDNSCounts(resp, 2, 0, 0)

	filtered := filterDNSAAAA(resp)
	if binary.BigEndian.Uint16(filtered[6:8]) != 1 {
		t.Fatalf("toggle ON: AAAA record must be stripped (ANCOUNT=1 expected)")
	}

	SetDisableIPv6(false)
	passthrough := filterDNSAAAA(resp)
	if &passthrough[0] != &resp[0] {
		t.Fatalf("toggle OFF: response must pass through unmodified (zero-copy)")
	}

	SetDisableIPv6(true)
	if !GetDisableIPv6() {
		t.Fatalf("SetDisableIPv6(true) must restore blocking")
	}
}

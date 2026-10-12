package dnscore

import (
	"encoding/binary"
	"testing"
)

func TestDNSStaticAndRefusal(t *testing.T) {
	good := Respond(makeQuery("hgfw.test"))
	if len(good) < 32 || binary.BigEndian.Uint16(good[6:8]) != 1 || good[3]&15 != 0 {
		t.Fatalf("no answer: %v", good)
	}
	refused := Respond(makeQuery("different.example"))
	if len(refused) < 12 || refused[3]&15 != 5 || binary.BigEndian.Uint16(refused[6:8]) != 0 {
		t.Fatal("nonlocal name answered")
	}
	aaa := makeQuery("hgfw.test")
	binary.BigEndian.PutUint16(aaa[len(aaa)-4:len(aaa)-2], 28)
	res := Respond(aaa)
	if res[3]&15 != 0 || binary.BigEndian.Uint16(res[6:8]) != 0 {
		t.Fatal("AAAA should be NODATA")
	}
	malformed := makeQuery("hgfw.test")
	malformed[12] = 0xc0
	if res = Respond(malformed); len(res) != 12 || res[3]&15 != 1 {
		t.Fatal("malformed compression not rejected")
	}
	huge := make([]byte, 513)
	if Respond(huge) != nil {
		t.Fatal("oversized DNS packet accepted")
	}
}
func TestDNSLoopbackSelfTest(t *testing.T) {
	r, err := SelfTest()
	if err != nil || !r.Success || !r.NoNetworkChanges {
		t.Fatalf("selftest %+v: %v", r, err)
	}
}

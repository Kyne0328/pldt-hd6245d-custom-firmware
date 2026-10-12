package dhcpcore

import (
	"encoding/binary"
	"net/netip"
	"testing"
	"time"
)

func config() Config {
	return Config{LAN: netip.MustParsePrefix("192.0.2.1/24"),
		Server: netip.MustParseAddr("192.0.2.1"),
		Start:  netip.MustParseAddr("192.0.2.100"),
		End:    netip.MustParseAddr("192.0.2.101"), LeaseTime: time.Hour}
}
func makePacket(mac [6]byte, kind byte, options ...byte) []byte {
	raw := make([]byte, 240)
	raw[0] = 1
	raw[1] = 1
	raw[2] = 6
	binary.BigEndian.PutUint32(raw[4:8], 0x10203040)
	copy(raw[28:34], mac[:])
	copy(raw[236:240], []byte{99, 130, 83, 99})
	raw = append(raw, 53, 1, kind)
	raw = append(raw, options...)
	return append(raw, 255)
}
func responseType(out []byte) byte {
	for i := 240; i < len(out); {
		b := out[i]
		i++
		if b == 255 {
			break
		}
		if b == 0 {
			continue
		}
		if i >= len(out) {
			return 0
		}
		n := int(out[i])
		i++
		if i+n > len(out) {
			return 0
		}
		if b == 53 && n == 1 {
			return out[i]
		}
		i += n
	}
	return 0
}
func TestIndependentDHCPCoreDiscoverRequestIsolation(t *testing.T) {
	core, err := New(config())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1800000000, 0)
	m1 := [6]byte{2, 1, 2, 3, 4, 5}
	m2 := [6]byte{2, 1, 2, 3, 4, 6}
	m3 := [6]byte{2, 1, 2, 3, 4, 7}
	discover := makePacket(m1, Discover)
	offer, err := core.Handle(discover, now)
	if err != nil || responseType(offer) != Offer || parseAddr(offer[16:20]).String() != "192.0.2.100" {
		t.Fatalf("first offer invalid: %v %v", err, offer)
	}
	request := makePacket(m1, Request, 50, 4, 192, 0, 2, 100, 54, 4, 192, 0, 2, 1)
	ack, err := core.Handle(request, now)
	if err != nil || responseType(ack) != ACK || parseAddr(ack[16:20]).String() != "192.0.2.100" {
		t.Fatalf("first ACK invalid: %v %v", err, ack)
	}
	offer2, err := core.Handle(makePacket(m2, Discover), now)
	if err != nil || responseType(offer2) != Offer || parseAddr(offer2[16:20]).String() != "192.0.2.101" {
		t.Fatalf("collision allowed: %v %v", err, offer2)
	}
	none, err := core.Handle(makePacket(m3, Discover), now)
	if err != nil || none != nil {
		t.Fatal("offered exhausted pool")
	}
	wrong, err := core.Handle(makePacket(m1, Request, 50, 4, 192, 0, 2, 100, 54, 4, 192, 0, 2, 55), now)
	if err != nil || wrong != nil {
		t.Fatal("answered request intended for different server")
	}
	bad, err := core.Handle(makePacket(m1, Request, 50, 4, 192, 0, 2, 101), now)
	if err != nil || responseType(bad) != NAK || parseAddr(bad[16:20]).String() != "0.0.0.0" {
		t.Fatalf("wrong-address request accepted %v", bad)
	}
	expired, err := core.Handle(makePacket(m3, Discover), now.Add(2*time.Hour))
	if err != nil || responseType(expired) != Offer {
		t.Fatal("expired allocation not reclaimed")
	}
}
func TestDHCPCoreRejectsMalformedAndUnsafeConfiguration(t *testing.T) {
	cfg := config()
	cfg.Server = netip.MustParseAddr("192.0.2.100")
	if _, e := New(cfg); e == nil {
		t.Fatal("gateway overlap accepted")
	}
	cfg = config()
	cfg.LeaseTime = 0
	if _, e := New(cfg); e == nil {
		t.Fatal("zero lease allowed")
	}
	cfg = config()
	core, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	mac := [6]byte{2, 1, 2, 3, 4, 5}
	raw := makePacket(mac, Discover)
	for _, mutate := range []func([]byte) []byte{
		func(b []byte) []byte { return b[:239] },
		func(b []byte) []byte { b[236] = 0; return b },
		func(b []byte) []byte { b[28] = 1; return b },
		func(b []byte) []byte { b[240] = 52; return b }, // overload not implemented
		func(b []byte) []byte { return append(b[:len(b)-1], 50, 4, 1) },
		func(b []byte) []byte { return append(b[:len(b)-1], 53, 1, Discover, 255) },
		func(b []byte) []byte { b[24] = 1; return b }, // relay agent unsupported
	} {
		clone := append([]byte(nil), raw...)
		b := mutate(clone)
		if out, err := core.Handle(b, time.Now()); err == nil || out != nil {
			t.Fatalf("accepted malformed DHCP packet: %x", b[:min(len(b), 30)])
		}
	}
}
func FuzzDHCPParseDoesNotPanic(f *testing.F) {
	f.Add(makePacket([6]byte{2, 1, 2, 3, 4, 5}, Discover))
	f.Add([]byte{1, 1, 6})
	f.Fuzz(func(t *testing.T, packet []byte) { _, _ = Parse(packet) })
}

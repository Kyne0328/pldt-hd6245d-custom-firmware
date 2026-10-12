// Package dhcpcore implements an independent, socket-free DHCPv4 transaction
// engine. It has NO live listener, subprocess, config writer or privileged path.
// This is an offline-tested protocol component, not an enabled LAN DHCP server.
package dhcpcore

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"sync"
	"time"
)

var ErrPacket = errors.New("invalid or unsupported DHCPv4 packet")

const (
	Discover byte = 1
	Offer    byte = 2
	Request  byte = 3
	ACK      byte = 5
	NAK      byte = 6
)

type RequestPacket struct {
	Kind      byte
	XID       uint32
	MAC       [6]byte
	Requested netip.Addr
	ServerID  netip.Addr
	ClientIP  netip.Addr
	Broadcast bool
}

func parseAddr(raw []byte) netip.Addr {
	if len(raw) != 4 {
		return netip.Addr{}
	}
	v := [4]byte{}
	copy(v[:], raw)
	return netip.AddrFrom4(v)
}
func Parse(raw []byte) (RequestPacket, error) {
	var req RequestPacket
	if len(raw) < 241 || len(raw) > 1500 || raw[0] != 1 || raw[1] != 1 || raw[2] != 6 || raw[3] > 16 ||
		string(raw[236:240]) != string([]byte{99, 130, 83, 99}) {
		return req, ErrPacket
	}
	// Relay agents and bootfile/option-overload transactions are not supported.
	for _, v := range raw[24:28] {
		if v != 0 {
			return req, ErrPacket
		}
	}
	copy(req.MAC[:], raw[28:34])
	if req.MAC[0]&1 != 0 || req.MAC == [6]byte{} {
		return req, ErrPacket
	}
	req.XID = binary.BigEndian.Uint32(raw[4:8])
	req.ClientIP = parseAddr(raw[12:16])
	req.Broadcast = binary.BigEndian.Uint16(raw[10:12])&0x8000 != 0
	foundKind := false
	seen50 := false
	seen54 := false
	finished := false
	for p := 240; p < len(raw); {
		op := raw[p]
		p++
		if op == 255 {
			finished = true
			break
		}
		if op == 0 {
			continue
		}
		if p >= len(raw) {
			return RequestPacket{}, ErrPacket
		}
		n := int(raw[p])
		p++
		if p+n > len(raw) {
			return RequestPacket{}, ErrPacket
		}
		value := raw[p : p+n]
		p += n
		switch op {
		case 53:
			if foundKind || n != 1 {
				return RequestPacket{}, ErrPacket
			}
			req.Kind = value[0]
			foundKind = true
		case 50:
			if seen50 || n != 4 {
				return RequestPacket{}, ErrPacket
			}
			req.Requested = parseAddr(value)
			seen50 = true
		case 54:
			if seen54 || n != 4 {
				return RequestPacket{}, ErrPacket
			}
			req.ServerID = parseAddr(value)
			seen54 = true
		case 52:
			return RequestPacket{}, ErrPacket // option overload unimplemented
		}
	}
	if !finished || !foundKind || (req.Kind != Discover && req.Kind != Request) {
		return RequestPacket{}, ErrPacket
	}
	return req, nil
}

type Config struct {
	LAN       netip.Prefix
	Server    netip.Addr
	Start     netip.Addr
	End       netip.Addr
	LeaseTime time.Duration
}

func (c Config) Validate() error {
	if !c.LAN.IsValid() || !c.LAN.Addr().Is4() || c.LAN.Bits() < 16 || c.LAN.Bits() > 30 ||
		!c.Server.Is4() || !c.Start.Is4() || !c.End.Is4() ||
		c.LeaseTime < time.Minute || c.LeaseTime > 7*24*time.Hour {
		return errors.New("invalid DHCP configuration")
	}
	network := c.LAN.Masked().Addr()
	bcast := network.As4()
	for bit := c.LAN.Bits(); bit < 32; bit++ {
		bcast[bit/8] |= 1 << uint(7-bit%8)
	}
	broadcast := netip.AddrFrom4(bcast)
	if !c.LAN.Contains(c.Server) || !c.LAN.Contains(c.Start) || !c.LAN.Contains(c.End) ||
		c.Server == network || c.Server == broadcast || c.Start == network || c.End == broadcast ||
		c.Start.Compare(c.End) > 0 || c.Start.Compare(c.Server) <= 0 && c.End.Compare(c.Server) >= 0 {
		return errors.New("DHCP pool overlaps reserved address")
	}
	// Bound processing cost and memory before a later networking integration.
	num := 0
	for ip := c.Start; ip.IsValid() && ip.Compare(c.End) <= 0; ip = ip.Next() {
		num++
		if num > 1024 {
			return errors.New("DHCP pool exceeds offline engine limit")
		}
	}
	return nil
}

type lease struct {
	IP      netip.Addr
	Until   time.Time
	Offered bool
}
type Engine struct {
	mu    sync.Mutex
	cfg   Config
	byMAC map[[6]byte]lease
}

// New only validates configuration and allocates in-process memory.
// It never opens a socket or changes any Linux network setting.
func New(cfg Config) (*Engine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Engine{cfg: cfg, byMAC: make(map[[6]byte]lease)}, nil
}
func nextIP(ip netip.Addr) netip.Addr { return ip.Next() }
func (e *Engine) allocate(mac [6]byte, now time.Time) (netip.Addr, bool) {
	// Expired offers/leases must not accumulate unbounded client identities.
	for old, v := range e.byMAC {
		if !v.Until.After(now) {
			delete(e.byMAC, old)
		}
	}
	if v, ok := e.byMAC[mac]; ok && v.Until.After(now) {
		return v.IP, true
	}
	for ip := e.cfg.Start; ip.IsValid() && ip.Compare(e.cfg.End) <= 0; ip = nextIP(ip) {
		used := false
		for oldMAC, v := range e.byMAC {
			if oldMAC != mac && v.IP == ip && v.Until.After(now) {
				used = true
				break
			}
		}
		if !used {
			return ip, true
		}
	}
	return netip.Addr{}, false
}
func buildResponse(raw []byte, cfg Config, kind byte, yiaddr netip.Addr) []byte {
	out := make([]byte, 240)
	copy(out, raw[:240])
	out[0] = 2
	out[3] = 0
	// The caller cannot copy network state from an untrusted request.
	for i := 12; i < 28; i++ {
		out[i] = 0
	}
	if yiaddr.Is4() {
		v := yiaddr.As4()
		copy(out[16:20], v[:])
	}
	server := cfg.Server.As4()
	copy(out[20:24], server[:])
	out = append(out, 53, 1, kind, 54, 4)
	out = append(out, server[:]...)
	if kind != NAK {
		secs := uint32(cfg.LeaseTime / time.Second)
		out = append(out, 51, 4, byte(secs>>24), byte(secs>>16), byte(secs>>8), byte(secs))
		mask := [4]byte{}
		for i := 0; i < cfg.LAN.Bits(); i++ {
			mask[i/8] |= 1 << uint(7-i%8)
		}
		out = append(out, 1, 4)
		out = append(out, mask[:]...)
		out = append(out, 3, 4)
		out = append(out, server[:]...)
	}
	out = append(out, 255)
	return out
}

// Handle synthesizes one OFFER/ACK/NAK; it never transmits it.
// It ignores DHCP requests selecting another server.
func (e *Engine) Handle(raw []byte, now time.Time) ([]byte, error) {
	req, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if req.ServerID.IsValid() && req.ServerID != e.cfg.Server {
		return nil, nil
	}
	switch req.Kind {
	case Discover:
		ip, ok := e.allocate(req.MAC, now)
		if !ok {
			return nil, nil
		}
		if old, ok := e.byMAC[req.MAC]; !ok || old.Offered || !old.Until.After(now) {
			e.byMAC[req.MAC] = lease{IP: ip, Until: now.Add(time.Minute), Offered: true}
		}
		return buildResponse(raw, e.cfg, Offer, ip), nil
	case Request:
		desired := req.Requested
		if !desired.IsValid() {
			desired = req.ClientIP
		}
		l, has := e.byMAC[req.MAC]
		valid := has && l.IP == desired && l.Until.After(now) && e.cfg.LAN.Contains(desired)
		if !valid {
			return buildResponse(raw, e.cfg, NAK, netip.Addr{}), nil
		}
		e.byMAC[req.MAC] = lease{IP: desired, Until: now.Add(e.cfg.LeaseTime)}
		return buildResponse(raw, e.cfg, ACK, desired), nil
	}
	return nil, ErrPacket
}

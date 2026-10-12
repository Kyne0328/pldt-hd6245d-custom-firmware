package dhcpcore

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"time"
)

// SelfTestResult documents an in-memory protocol exchange only.
// No UDP, raw socket, interface, service or DHCP broadcast is used.
type SelfTestResult struct {
	Success               bool `json:"success"`
	SocketFree            bool `json:"socketFree"`
	OfferCorrect          bool `json:"offerCorrect"`
	ACKCorrect            bool `json:"ackCorrect"`
	UnknownClientRejected bool `json:"unknownClientRejected"`
}

func typeOfReply(raw []byte) byte {
	if len(raw) < 243 {
		return 0
	}
	for p := 240; p < len(raw); {
		op := raw[p]
		p++
		if op == 255 {
			break
		}
		if op == 0 {
			continue
		}
		if p >= len(raw) {
			break
		}
		n := int(raw[p])
		p++
		if p+n > len(raw) {
			break
		}
		if op == 53 && n == 1 {
			return raw[p]
		}
		p += n
	}
	return 0
}
func syntheticRequest(mac [6]byte, kind byte, options ...byte) []byte {
	raw := make([]byte, 240)
	raw[0], raw[1], raw[2] = 1, 1, 6
	binary.BigEndian.PutUint32(raw[4:8], 0x12345678)
	copy(raw[28:34], mac[:])
	copy(raw[236:240], []byte{99, 130, 83, 99})
	raw = append(raw, 53, 1, kind)
	raw = append(raw, options...)
	return append(raw, 255)
}
func SelfTest() (SelfTestResult, error) {
	result := SelfTestResult{SocketFree: true}
	cfg := Config{LAN: netip.MustParsePrefix("192.0.2.1/24"), Server: netip.MustParseAddr("192.0.2.1"),
		Start: netip.MustParseAddr("192.0.2.100"), End: netip.MustParseAddr("192.0.2.101"), LeaseTime: time.Hour}
	engine, err := New(cfg)
	if err != nil {
		return result, err
	}
	now := time.Unix(1800000000, 0)
	m1 := [6]byte{2, 1, 2, 3, 4, 5}
	m2 := [6]byte{2, 1, 2, 3, 4, 6}
	offer, err := engine.Handle(syntheticRequest(m1, Discover), now)
	if err != nil {
		return result, err
	}
	result.OfferCorrect = typeOfReply(offer) == Offer && len(offer) >= 20 && parseAddr(offer[16:20]) == cfg.Start
	ack, err := engine.Handle(syntheticRequest(m1, Request, 50, 4, 192, 0, 2, 100, 54, 4, 192, 0, 2, 1), now)
	if err != nil {
		return result, err
	}
	result.ACKCorrect = typeOfReply(ack) == ACK && len(ack) >= 20 && parseAddr(ack[16:20]) == cfg.Start
	nak, err := engine.Handle(syntheticRequest(m2, Request, 50, 4, 192, 0, 2, 100), now)
	if err != nil {
		return result, err
	}
	result.UnknownClientRejected = typeOfReply(nak) == NAK
	result.Success = result.OfferCorrect && result.ACKCorrect && result.UnknownClientRejected
	if !result.Success {
		return result, errors.New("in-memory DHCP protocol self-test failed")
	}
	return result, nil
}

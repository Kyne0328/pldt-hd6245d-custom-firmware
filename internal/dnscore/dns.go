// Package dnscore implements a minimal first-party static DNS responder for
// offline testing. It never binds port 53, changes resolv.conf, or forwards
// queries. LAN production activation is intentionally not exposed.
package dnscore

import (
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"time"
)

const localName = "hgfw.test"

func parseQuestion(raw []byte) (name string, end int, ok bool) {
	if len(raw) < 17 || len(raw) > 512 || binary.BigEndian.Uint16(raw[4:6]) != 1 ||
		binary.BigEndian.Uint16(raw[6:8]) != 0 || binary.BigEndian.Uint16(raw[8:10]) != 0 ||
		binary.BigEndian.Uint16(raw[10:12]) != 0 {
		return "", 0, false
	}
	offset := 12
	labels := []string{}
	total := 0
	for {
		if offset >= len(raw) {
			return "", 0, false
		}
		n := int(raw[offset])
		offset++
		if n == 0 {
			break
		}
		if n > 63 || n&0xc0 != 0 || offset+n > len(raw) {
			return "", 0, false
		}
		v := string(raw[offset : offset+n])
		for _, c := range v {
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' {
				return "", 0, false
			}
		}
		labels = append(labels, strings.ToLower(v))
		total += n + 1
		if total > 253 || len(labels) > 16 {
			return "", 0, false
		}
		offset += n
	}
	if offset+4 != len(raw) {
		return "", 0, false
	}
	return strings.Join(labels, "."), offset + 4, true
}

func Respond(query []byte) []byte {
	if len(query) < 12 || len(query) > 512 {
		return nil
	}
	response := make([]byte, 12)
	copy(response[0:2], query[0:2])
	flags := uint16(0x8000) | binary.BigEndian.Uint16(query[2:4])&0x0100
	// Refuse non-standard opcodes and all malformed queries.
	name, end, ok := parseQuestion(query)
	if !ok || binary.BigEndian.Uint16(query[2:4])&0x7800 != 0 {
		binary.BigEndian.PutUint16(response[2:4], flags|1) // FORMERR
		return response
	}
	typ := binary.BigEndian.Uint16(query[end-4 : end-2])
	class := binary.BigEndian.Uint16(query[end-2 : end])
	if name != localName || class != 1 {
		binary.BigEndian.PutUint16(response[2:4], flags|5) // REFUSED, never forward
	} else {
		flags |= 0x0400 // authoritative for this one static record
		binary.BigEndian.PutUint16(response[2:4], flags)
		if typ == 1 {
			binary.BigEndian.PutUint16(response[6:8], 1)
		}
	}
	binary.BigEndian.PutUint16(response[4:6], 1)
	response = append(response, query[12:end]...)
	if name == localName && class == 1 && typ == 1 {
		// Compression pointer refers to the sole question's QNAME at offset 12.
		response = append(response, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 30, 0, 4, 127, 0, 0, 1)
	}
	return response
}

type Result struct {
	Success            bool `json:"success"`
	LoopbackOnly       bool `json:"loopbackOnly"`
	StaticAResolved    bool `json:"staticAResolved"`
	ForeignNameRefused bool `json:"foreignNameRefused"`
	NoNetworkChanges   bool `json:"noNetworkChanges"`
}

func makeQuery(name string) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint16(b[0:2], 0x42c1)
	binary.BigEndian.PutUint16(b[2:4], 0x0100)
	binary.BigEndian.PutUint16(b[4:6], 1)
	for _, s := range strings.Split(name, ".") {
		b = append(b, byte(len(s)))
		b = append(b, s...)
	}
	b = append(b, 0, 0, 1, 0, 1)
	return b
}

// SelfTest binds an ephemeral UDP port on 127.0.0.1, answers two local
// synthetic requests and stops. No external DNS traffic or daemon is started.
func SelfTest() (Result, error) {
	r := Result{NoNetworkChanges: true}
	listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		return r, err
	}
	defer listener.Close()
	if !listener.LocalAddr().(*net.UDPAddr).IP.IsLoopback() {
		return r, errors.New("nonloopback bind")
	}
	r.LoopbackOnly = true
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 512)
		for i := 0; i < 2; i++ {
			_ = listener.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, addr, e := listener.ReadFrom(buf)
			if e != nil {
				done <- e
				return
			}
			if !addr.(*net.UDPAddr).IP.IsLoopback() {
				done <- errors.New("remote address")
				return
			}
			response := Respond(buf[:n])
			if _, e = listener.WriteTo(response, addr); e != nil {
				done <- e
				return
			}
		}
		done <- nil
	}()
	client, err := net.DialTimeout("udp4", listener.LocalAddr().String(), time.Second)
	if err != nil {
		return r, err
	}
	defer client.Close()
	for i, name := range []string{localName, "untrusted.example"} {
		_ = client.SetDeadline(time.Now().Add(2 * time.Second))
		if _, err = client.Write(makeQuery(name)); err != nil {
			return r, err
		}
		b := make([]byte, 512)
		var n int
		if n, err = client.Read(b); err != nil {
			return r, err
		}
		if n < 12 {
			return r, errors.New("short reply")
		}
		flags := binary.BigEndian.Uint16(b[2:4])
		if i == 0 {
			r.StaticAResolved = flags&0xf == 0 && binary.BigEndian.Uint16(b[6:8]) == 1 && n >= 16 && string(b[n-4:n]) == string([]byte{127, 0, 0, 1})
		}
		if i == 1 {
			r.ForeignNameRefused = flags&0xf == 5 && binary.BigEndian.Uint16(b[6:8]) == 0
		}
	}
	if err = <-done; err != nil {
		return r, err
	}
	r.Success = r.LoopbackOnly && r.StaticAResolved && r.ForeignNameRefused
	if !r.Success {
		return r, errors.New("DNS loopback response mismatch")
	}
	return r, nil
}

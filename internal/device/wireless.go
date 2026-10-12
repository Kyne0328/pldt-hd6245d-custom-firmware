package device

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// WirelessObservation records evidence from read-only kernel interfaces and
// optional driver information utilities. A driver unavailable != zero clients.
type WirelessObservation struct {
	Radios []WirelessRadio `json:"radios"`
	Notes  []string        `json:"notes"`
}
type WirelessRadio struct {
	Interface         string            `json:"interface"`
	Label             string            `json:"label"`
	Master            string            `json:"master,omitempty"`
	OperState         string            `json:"operState,omitempty"`
	ObservedSSID      string            `json:"observedSSID,omitempty"`
	Channel           string            `json:"channel,omitempty"`
	Mode              string            `json:"mode,omitempty"`
	Source            string            `json:"source"`
	AssociationStatus string            `json:"associationStatus"`
	Stations          []WirelessStation `json:"stations"`
}
type WirelessStation struct {
	MAC       string `json:"mac"`
	SignalDBm *int   `json:"signalDBm,omitempty"`
	RXBitrate string `json:"rxBitrate,omitempty"`
	TXBitrate string `json:"txBitrate,omitempty"`
	// The IPv4 address is separately observed from ARP, never inferred as a DHCP lease.
	NeighborIPv4      string `json:"neighborIPv4,omitempty"`
	NeighborInterface string `json:"neighborInterface,omitempty"`
	LeaseHostname     string `json:"leaseHostname,omitempty"`
	Source            string `json:"source"`
}

var macMatcher = regexp.MustCompile(`(?i)\b(?:[0-9a-f]{2}:){5}[0-9a-f]{2}\b`)
var channelMatcher = regexp.MustCompile(`(?i)\bchannel\s*:?\s*([0-9]{1,3})\b`)
var validSSIDLine = regexp.MustCompile(`(?im)^\s*ssid\s*:?\s+(.*?)\s*$`)
var numericRSSI = regexp.MustCompile(`^\s*(-?[0-9]{1,3})(?:\s|$)`)

type wifiRunner func(ctx context.Context, command string, args ...string) (string, error)

func wifiExec(ctx context.Context, command string, args ...string) (string, error) {
	path, err := exec.LookPath(command)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, path, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	// Avoid inherited stdin and never run through a shell.
	cmd.Stdin = nil
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		return "", err
	}
	b, readErr := io.ReadAll(io.LimitReader(stdout, 16385))
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if readErr != nil {
		return "", readErr
	}
	if len(b) > 16384 {
		return "", errors.New("wireless driver output exceeded limit")
	}
	if waitErr != nil {
		return "", waitErr
	}
	return string(b), nil
}
func wifiCallContext(parent context.Context, run wifiRunner, bin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 900*time.Millisecond)
	defer cancel()
	return run(ctx, bin, args...)
}
func wifiCall(run wifiRunner, bin string, args ...string) (string, error) {
	return wifiCallContext(context.Background(), run, bin, args...)
}
func normalMAC(m string) string {
	mac, err := net.ParseMAC(m)
	if err != nil || len(mac) != 6 {
		return ""
	}
	return strings.ToLower(mac.String())
}
func parseWLAssociations(data string) []WirelessStation {
	out := []WirelessStation{}
	seen := map[string]bool{}
	// The Broadcom wl syntax prints 'assoclist <MAC>' or 'assoclist: <MAC>'.
	for _, line := range strings.Split(data, string([]byte{10})) {
		trimmed := strings.ToLower(strings.TrimSpace(line))
		if !strings.HasPrefix(trimmed, "assoclist") {
			continue
		}
		m := macMatcher.FindString(line)
		mac := normalMAC(m)
		if mac == "" || seen[mac] || len(out) >= 64 {
			continue
		}
		seen[mac] = true
		out = append(out, WirelessStation{MAC: mac, Source: "Broadcom wl assoclist"})
	}
	return out
}

// Some driver helpers print an error on stdout yet exit with code zero.
// A failed association query must not masquerade as zero connected clients.
func validWLAssociationOutput(data string) bool {
	for _, line := range strings.Split(data, "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		if line == "" || line == "assoclist" || line == "assoclist:" {
			continue
		}
		if !(strings.HasPrefix(line, "assoclist ") || strings.HasPrefix(line, "assoclist: ")) || normalMAC(macMatcher.FindString(line)) == "" {
			return false
		}
	}
	return true
}
func parseIWStations(data string) []WirelessStation {
	out := []WirelessStation{}
	seen := map[string]bool{}
	var current *WirelessStation
	flush := func() {
		if current != nil && current.MAC != "" && !seen[current.MAC] && len(out) < 64 {
			seen[current.MAC] = true
			out = append(out, *current)
		}
	}
	for _, line := range strings.Split(data, string([]byte{10})) {
		stripped := strings.TrimSpace(line)
		if strings.HasPrefix(stripped, "Station ") {
			flush()
			mac := normalMAC(macMatcher.FindString(strings.TrimPrefix(stripped, "Station ")))
			if mac == "" {
				current = nil
			} else {
				current = &WirelessStation{MAC: mac, Source: "nl80211 iw station dump"}
			}
			continue
		}
		if current == nil {
			continue
		}
		lower := strings.ToLower(stripped)
		if strings.HasPrefix(lower, "signal:") {
			text := strings.TrimSpace(strings.TrimPrefix(lower, "signal:"))
			x := numericRSSI.FindStringSubmatch(text)
			if len(x) > 1 {
				n, e := strconv.Atoi(x[1])
				if e == nil && n >= -130 && n <= 20 {
					current.SignalDBm = &n
				}
			}
		}
		for _, f := range []struct {
			key  string
			dest *string
		}{{"rx bitrate:", &current.RXBitrate}, {"tx bitrate:", &current.TXBitrate}} {
			if strings.HasPrefix(lower, f.key) {
				raw := strings.TrimSpace(stripped[len(f.key):])
				if len(raw) > 80 {
					raw = raw[:80]
				}
				*f.dest = raw
			}
		}
	}
	flush()
	return out
}

// An SSID is at most 32 bytes (not 32 runes). Reject diagnostic text and
// malformed quoting rather than accidentally displaying it as a network name.
func parseSSIDValue(raw string) string {
	value := strings.TrimSpace(raw)
	if strings.HasPrefix(value, "\"") {
		if len(value) < 2 || !strings.HasSuffix(value, "\"") {
			return ""
		}
		value = value[1 : len(value)-1]
	} else if strings.Contains(value, "\"") {
		return ""
	}
	if len(value) == 0 || len(value) > 32 {
		return ""
	}
	for _, r := range value {
		if r < 32 || r == 127 {
			return ""
		}
	}
	return value
}
func parseWLField(output, field string) string {
	text := strings.TrimSpace(output)
	if field == "ssid" {
		if strings.ContainsAny(text, "\r\n") {
			return ""
		}
		switch {
		case strings.HasPrefix(strings.ToLower(text), "current ssid:"):
			text = strings.TrimSpace(text[len("current ssid:"):])
		case strings.HasPrefix(strings.ToLower(text), "ssid:"):
			text = strings.TrimSpace(text[len("ssid:"):])
		default:
			return ""
		}
		return parseSSIDValue(text)
	}
	if match := channelMatcher.FindStringSubmatch(text); len(match) > 1 {
		return match[1]
	}
	return ""
}
func parseIWInfo(info string) (ssid, channel, mode string) {
	if m := validSSIDLine.FindStringSubmatch(info); len(m) > 1 {
		ssid = parseSSIDValue(m[1])
	}
	if m := channelMatcher.FindStringSubmatch(info); len(m) > 1 {
		channel = m[1]
	}
	for _, l := range strings.Split(info, string([]byte{10})) {
		v := strings.TrimSpace(l)
		if strings.HasPrefix(v, "type ") {
			mode = strings.TrimSpace(strings.TrimPrefix(v, "type "))
			if len(mode) > 36 {
				mode = ""
			}
		}
	}
	return
}

// InspectWireless is read-only, allowlisted and bounded. It deliberately
// refuses to classify ARP/DHCP clients as Wi-Fi association records.
func InspectWireless(model string, network NetworkInspection, run wifiRunner, lookup func(string) (string, error)) WirelessObservation {
	info := WirelessObservation{Radios: []WirelessRadio{}, Notes: []string{
		"Stations are reported ONLY from a successful driver association query; an absent helper means status unavailable, never zero clients.",
		"A reported wireless association is not a DHCP lease or internet connection.",
		"Station signal/rates are best-effort measurements, not proof of internet throughput. Missing values are unavailable, not zero.",
		"No SSID, radio power, AP/STA mode, password, bridge or network setting is modified.",
	}}
	evidence := map[string]NetworkEvidence{}
	for _, e := range network.Interfaces {
		evidence[e.Name] = e
	}
	// Native Broadcom driver wl interfaces are known on this BCM96846 board.
	// Do not probe unknown devices just because their interface starts wl.
	if !strings.Contains(strings.ToLower(model), "bcm96846") {
		return info
	}
	_, wlErr := lookup("wl")
	_, iwErr := lookup("iw")
	for _, name := range []string{"wl0", "wl1"} {
		e, present := evidence[name]
		if !present {
			continue
		}
		label := "2.4 GHz (board mapping)"
		if name == "wl1" {
			label = "5 GHz (board mapping)"
		}
		r := WirelessRadio{Interface: name, Label: label, Master: e.Master, OperState: e.OperState, Source: "Linux kernel interfaces", AssociationStatus: "unavailable", Stations: []WirelessStation{}}
		// Prefer the genuine proprietary wl association list if installed; fallback
		// to standard nl80211 read-only iw when wl is missing/not supported.
		if wlErr == nil {
			if output, err := wifiCall(run, "wl", "-i", name, "assoclist"); err == nil && validWLAssociationOutput(output) {
				r.Stations = parseWLAssociations(output)
				r.AssociationStatus = "observed"
				r.Source = "Broadcom wl assoclist"
			}
			if output, err := wifiCall(run, "wl", "-i", name, "ssid"); err == nil {
				r.ObservedSSID = parseWLField(output, "ssid")
			}
			if output, err := wifiCall(run, "wl", "-i", name, "channel"); err == nil {
				r.Channel = parseWLField(output, "channel")
			}
		}
		if iwErr == nil && r.AssociationStatus != "observed" {
			if output, err := wifiCall(run, "iw", "dev", name, "station", "dump"); err == nil {
				r.Stations = parseIWStations(output)
				r.AssociationStatus = "observed"
				r.Source = "nl80211 iw station dump"
			}
		}
		if iwErr == nil && (r.ObservedSSID == "" || r.Channel == "" || r.Mode == "") {
			if output, err := wifiCall(run, "iw", "dev", name, "info"); err == nil {
				ssid, ch, mode := parseIWInfo(output)
				if r.ObservedSSID == "" {
					r.ObservedSSID = ssid
				}
				if r.Channel == "" {
					r.Channel = ch
				}
				r.Mode = mode
			}
		}
		info.Radios = append(info.Radios, r)
	}
	if wlErr == nil {
		enrichWLStations(info.Radios, run)
	}
	return info
}
func SnapshotWireless(model string, n NetworkInspection) WirelessObservation {
	return InspectWireless(model, n, wifiExec, exec.LookPath)
}
func (w WirelessObservation) Summary() string {
	observed := 0
	count := 0
	for _, r := range w.Radios {
		if r.AssociationStatus == "observed" {
			observed++
			count += len(r.Stations)
		}
	}
	return fmt.Sprintf("%d radios, %d association inventories available, %d associated stations observed", len(w.Radios), observed, count)
}

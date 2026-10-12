package device

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWLRSSIAndStationRates(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int
		ok   bool
	}{
		{"-52\n", -52, true}, {"RSSI: -68 dBm", -68, true},
		{"0", 0, false}, {"-131", 0, false}, {"190", 0, false},
		{"per antenna rssi: -31 -40", 0, false}, {"noise -53", 0, false},
		{"RSSI: -43\nERROR", 0, false},
	} {
		got := parseWLRSSI(tc.raw)
		if (got != nil) != tc.ok || (tc.ok && *got != tc.want) {
			t.Errorf("parseWLRSSI(%q) = %v, want %d ok=%v", tc.raw, got, tc.want, tc.ok)
		}
	}
	rx, tx := parseWLStationRates("STA 02:11:22:33:44:55:\n" +
		"rateset [ 1 2 5.5 11 54 ]\n" +
		"rate of last tx pkt: 78000 kbps\n" +
		"rate of last rx pkt: 24000 kbps\n" +
		"per antenna average rssi of rx data frames: -18 -27 -26 0\n")
	if rx != "24 MBit/s" || tx != "78 MBit/s" {
		t.Fatalf("packet rates rx=%q tx=%q", rx, tx)
	}
	// Observed on the physical HG6245D: the TX field has two numeric rates.
	// The secondary number is not identified, so it must not replace the first.
	for _, tc := range []struct{ output, wantRX, wantTX string }{
		{"  rate of last tx pkt: 292500 kbps - 97500 kbps\n  rate of last rx pkt: 390000 kbps", "390 MBit/s", "292.5 MBit/s"},
		{"rate of last tx pkt: 195000 kbps - 45000 kbps\nrate of last rx pkt: 6000 kbps", "6 MBit/s", "195 MBit/s"},
		{"rate of last tx pkt: 1.5 Gbps - 72.2 Mbps\nrate of last rx pkt: 24 Mbps", "24 MBit/s", "1500 MBit/s"},
	} {
		rx, tx = parseWLStationRates(tc.output)
		if rx != tc.wantRX || tx != tc.wantTX {
			t.Errorf("paired packet rate rx=%q tx=%q, want rx=%q tx=%q", rx, tx, tc.wantRX, tc.wantTX)
		}
	}
	for _, bad := range []string{
		"rate of last tx pkt: 292500 kbps - ERROR",
		"rate of last tx pkt: 292500 kbps - 97500",
		"rate of last tx pkt: 292500 kbps - 97500 kbps nonsense",
		"rate of last tx pkt: 292500 kbps - -45000 kbps",
	} {
		_, tx = parseWLStationRates(bad)
		if tx != "" {
			t.Errorf("malformed supplemental rate %q accepted as %q", bad, tx)
		}
	}
	rx, tx = parseWLStationRates("rate of last tx pkt: 1.5 Gbps\nrate of last rx pkt: 72.2 Mbps\nrate of last rx pkt: 999999 kbps\n")
	if rx != "999.999 MBit/s" || tx != "1500 MBit/s" {
		t.Fatalf("mixed units rx=%q tx=%q", rx, tx)
	}
	rx, tx = parseWLStationRates("rateset [54]\nrate of last tx pkt: 99999999 Mbps\nrate of last rx pkt: -10 kbps\n")
	if rx != "" || tx != "" {
		t.Fatalf("invalid packet rates rx=%q tx=%q", rx, tx)
	}
}

func TestWirelessEnrichmentReadOnlyAndBounded(t *testing.T) {
	n := NetworkInspection{Interfaces: []NetworkEvidence{{Name: "wl0"}, {Name: "wl1"}}}
	var calls []string
	runner := func(ctx context.Context, command string, args ...string) (string, error) {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		full := command + " " + strings.Join(args, " ")
		calls = append(calls, full)
		if command != "wl" {
			return "", errors.New("unsupported")
		}
		switch args[2] {
		case "assoclist":
			return "assoclist 02:11:22:33:44:55\n", nil
		case "ssid":
			return "Current SSID: \"Test WiFi\"", nil
		case "channel":
			return "Current channel: 36", nil
		case "sta_info":
			return "rate of last tx pkt: 72000 kbps\nrate of last rx pkt: 13000 kbps\n", nil
		case "rssi":
			return "-55", nil
		}
		return "", errors.New("unsupported")
	}
	lookup := func(name string) (string, error) {
		if name == "wl" {
			return "/bin/wl", nil
		}
		return "", errors.New("not found")
	}
	out := InspectWireless("BCM96846", n, runner, lookup)
	for _, r := range out.Radios {
		if len(r.Stations) != 1 || r.Stations[0].SignalDBm == nil || *r.Stations[0].SignalDBm != -55 || r.Stations[0].RXBitrate != "13 MBit/s" || r.Stations[0].TXBitrate != "72 MBit/s" || r.ObservedSSID != "Test WiFi" {
			t.Fatalf("incomplete enrichment: %+v", r)
		}
	}
	for _, call := range calls {
		if strings.Contains(call, " up") || strings.Contains(call, " down") || strings.Contains(call, " set") {
			t.Fatalf("unexpected mutating command: %s", call)
		}
	}
	// Unsupported detail commands must never hide a valid association inventory.
	unknown := InspectWireless("BCM96846", n, func(ctx context.Context, cmd string, args ...string) (string, error) {
		if args[2] == "assoclist" {
			return "assoclist 02:11:22:33:44:55", nil
		}
		return "", errors.New("unsupported")
	}, lookup)
	for _, r := range unknown.Radios {
		if r.AssociationStatus != "observed" || len(r.Stations) != 1 || r.Stations[0].SignalDBm != nil || r.Stations[0].RXBitrate != "" || r.Stations[0].TXBitrate != "" {
			t.Fatalf("unsupported details misreported: %+v", r)
		}
	}
}

func TestWLAssociationErrorCannotMasqueradeAsNoClients(t *testing.T) {
	for _, output := range []string{"wl: unsupported", "assoclist: error reading driver", "ERROR: failed", "assoclist ff:gg:hh:ii:jj:kk"} {
		if validWLAssociationOutput(output) {
			t.Fatalf("invalid association output was accepted: %q", output)
		}
	}
	for _, output := range []string{"", "assoclist:", "assoclist 02:11:22:33:44:55", "assoclist: 02:11:22:33:44:55"} {
		if !validWLAssociationOutput(output) {
			t.Fatalf("valid association output was rejected: %q", output)
		}
	}
	iface := NetworkInspection{Interfaces: []NetworkEvidence{{Name: "wl0"}}}
	actual := InspectWireless("BCM96846", iface, func(_ context.Context, _ string, args ...string) (string, error) {
		if args[len(args)-1] == "assoclist" {
			return "wl: unsupported", nil
		}
		return "", errors.New("unsupported")
	}, func(name string) (string, error) {
		if name == "wl" {
			return "/bin/wl", nil
		}
		return "", errors.New("missing")
	})
	if len(actual.Radios) != 1 || actual.Radios[0].AssociationStatus != "unavailable" || len(actual.Radios[0].Stations) != 0 {
		t.Fatalf("failed query misreported: %+v", actual.Radios)
	}
}

func TestWLSSIDRejectsAmbiguousOutput(t *testing.T) {
	for _, input := range []string{"I", "wl status: ready", "Current SSID: \"unclosed", "Current SSID: \"abc\"\nERROR", "SSID: \"nested\" garbage", "SSID: " + strings.Repeat("x", 33)} {
		if got := parseWLField(input, "ssid"); got != "" {
			t.Errorf("ambiguous SSID %q accepted as %q", input, got)
		}
	}
	// One-character SSIDs are valid; use a synthetic value in tests rather
	// than the owner's actual network name.
	if got := parseWLField("Current SSID: \"Q\"", "ssid"); got != "Q" {
		t.Fatalf("single-character SSID rejected: %q", got)
	}
	if got := parseWLField("SSID: Cafe WiFi", "ssid"); got != "Cafe WiFi" {
		t.Fatalf("unquoted SSID rejected: %q", got)
	}
}

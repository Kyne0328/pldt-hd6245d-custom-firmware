package device

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestBroadcomStationRecordsOnly(t *testing.T) {
	list := parseWLAssociations("assoclist 02:11:22:33:44:55\nassoclist 02:11:22:33:44:55\nother ff:ff:ff:ff:ff:ff\nassoclist: a0:b1:c2:d3:e4:f5\n")
	if len(list) != 2 || list[0].MAC != "02:11:22:33:44:55" || list[1].MAC != "a0:b1:c2:d3:e4:f5" {
		t.Fatalf("station list invalid: %+v", list)
	}
	iw := parseIWStations("Station 02:11:22:33:44:55 (on wlan0)\n\tsignal: -46 dBm\n\trx bitrate: 72.2 MBit/s\n\ttx bitrate: 65.0 MBit/s\nStation a0:b1:c2:d3:e4:f5 (on wlan0)\n\tsignal: -61 dBm\n")
	if len(iw) != 2 || iw[0].SignalDBm == nil || *iw[0].SignalDBm != -46 || iw[0].TXBitrate != "65.0 MBit/s" {
		t.Fatalf("iw parsed incorrectly: %+v", iw)
	}
}
func TestWirelessHardwareReadOnlyAndUnavailability(t *testing.T) {
	iface := NetworkInspection{Interfaces: []NetworkEvidence{{Name: "wl0", Master: "br0"}, {Name: "wl1", Master: "br0"}}}
	commands := []string{}
	runner := func(_ context.Context, command string, args ...string) (string, error) {
		commands = append(commands, command+" "+strings.Join(args, " "))
		if command == "wl" {
			switch args[len(args)-1] {
			case "assoclist":
				if args[1] == "wl1" {
					return "assoclist 02:11:22:33:44:55", nil
				}
				return "", nil
			case "ssid":
				return "SSID: LabNetwork", nil
			case "channel":
				return "Current channel: 36", nil
			}
		}
		return "", errors.New("unsupported")
	}
	lookup := func(name string) (string, error) {
		if name == "wl" {
			return "/bin/wl", nil
		}
		return "", errors.New("unavailable")
	}
	actual := InspectWireless("Broadcom BCM96846", iface, runner, lookup)
	if len(actual.Radios) != 2 || actual.Radios[0].AssociationStatus != "observed" || len(actual.Radios[0].Stations) != 0 || len(actual.Radios[1].Stations) != 1 || actual.Radios[0].ObservedSSID != "LabNetwork" || actual.Radios[1].Channel != "36" {
		t.Fatalf("wireless wrong: %+v", actual)
	}
	for _, c := range commands {
		if strings.Contains(c, " up") || strings.Contains(c, " down") || strings.Contains(c, " set") || strings.Contains(c, " start") {
			t.Fatalf("possible mutating command: %s", c)
		}
	}
	no := InspectWireless("Broadcom BCM96846", iface, runner, func(string) (string, error) { return "", errors.New("not found") })
	for _, rad := range no.Radios {
		if rad.AssociationStatus != "unavailable" || len(rad.Stations) != 0 {
			t.Fatalf("unknown misreported as zero clients %+v", rad)
		}
	}
	if len(InspectWireless("unrelated linux", iface, runner, lookup).Radios) != 0 {
		t.Fatal("probed unknown hardware")
	}
}
func TestIWInfo(t *testing.T) {
	ssid, channel, mode := parseIWInfo("Interface wlan0\n\ttype AP\n\tssid LabNetwork\n\tchannel 11 (2462 MHz)\n")
	if ssid != "LabNetwork" || channel != "11" || mode != "AP" {
		t.Fatalf("unexpected iw parsing %q %q %q", ssid, channel, mode)
	}
	if parseWLField("SSID: \"Cafe WiFi\"", "ssid") != "Cafe WiFi" {
		t.Fatal("ssid")
	}
	if parseWLField("Current channel: 149", "channel") != "149" {
		t.Fatal("channel")
	}
}

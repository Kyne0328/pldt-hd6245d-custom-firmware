package fieldcheck

import (
	"encoding/json"
	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/device"
	"strings"
	"testing"
)

func TestFieldCheckSummaryDoesNotDiscloseIdentifiers(t *testing.T) {
	r := device.WirelessRadio{Interface: "wl1", AssociationStatus: "observed", ObservedSSID: "SECRET-NET", Stations: []device.WirelessStation{{MAC: "02:11:22:33:44:55", RXBitrate: "6 MBit/s", TXBitrate: "195 MBit/s", NeighborIPv4: "192.168.1.9"}}}
	s := device.Status{Interfaces: []device.Interface{{Name: "wl1", MAC: "02:11:22:33:44:55"}}, Wireless: device.WirelessObservation{Radios: []device.WirelessRadio{r}}, ARPNeighbors: []device.ARPNeighbor{{Address: "192.168.1.9"}}, ServiceSources: device.ServiceDiscovery{ResolverCandidates: []device.SourceProbe{{Label: "/etc/resolv.conf", Present: true}}, LeaseCandidates: []device.SourceProbe{{Label: "/tmp/leases", Present: true}}, KnownProcessNames: []string{"dnsmasq"}, DNSUDP53Observed: true}}
	inventory := Summarize(s)
	if inventory.AssociatedStations != 1 || inventory.StationsWithTX != 1 || inventory.StationsWithRX != 1 || inventory.MatchedWirelessARP != 1 {
		t.Fatalf("incorrect counts %+v", inventory)
	}
	b, err := json.Marshal(Result{ReadOnly: true, Inventory: inventory})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SECRET-NET", "02:11:22:33:44:55", "192.168.1.9"} {
		if strings.Contains(string(b), secret) {
			t.Fatalf("private information leaked: %s", secret)
		}
	}
}

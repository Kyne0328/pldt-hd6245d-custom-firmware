package device

import "testing"

func TestAssociateARPOnlyWithIndependentDriverAndBridgeEvidence(t *testing.T) {
	radios := []WirelessRadio{
		{Interface: "wl0", Master: "br0", AssociationStatus: "observed", Stations: []WirelessStation{{MAC: "02:11:22:33:44:55"}, {MAC: "02:11:22:33:44:66"}, {MAC: "02:11:22:33:44:77"}}},
		{Interface: "wl1", Master: "br0", AssociationStatus: "unavailable", Stations: []WirelessStation{{MAC: "02:11:22:33:44:99"}}},
	}
	neighbors := []ARPNeighbor{
		{MAC: "02:11:22:33:44:55", Address: "192.168.1.17", Interface: "br0"},
		{MAC: "02:11:22:33:44:66", Address: "192.168.1.18", Interface: "br0"},
		{MAC: "02:11:22:33:44:66", Address: "192.168.1.19", Interface: "br0"},  // conflicting IPs
		{MAC: "02:11:22:33:44:77", Address: "192.168.1.20", Interface: "wan0"}, // wrong L2
		{MAC: "02:11:22:33:44:99", Address: "192.168.1.21", Interface: "br0"},
	}
	AssociateARP(radios, neighbors)
	if radios[0].Stations[0].NeighborIPv4 != "192.168.1.17" || radios[0].Stations[0].NeighborInterface != "br0" {
		t.Fatal("known bridge observation missing")
	}
	for _, s := range []WirelessStation{radios[0].Stations[1], radios[0].Stations[2], radios[1].Stations[0]} {
		if s.NeighborIPv4 != "" {
			t.Fatalf("fabricated or ambiguous IP: %+v", s)
		}
	}
	if n := CountARPLinks(radios); n != 1 {
		t.Fatalf("unexpected count %d", n)
	}
}
func TestAssociateARPNoBridgeNoGuess(t *testing.T) {
	r := []WirelessRadio{{Interface: "wl0", AssociationStatus: "observed", Stations: []WirelessStation{{MAC: "02:11:22:33:44:55"}}}}
	AssociateARP(r, []ARPNeighbor{{Address: "10.0.0.5", MAC: "02:11:22:33:44:55", Interface: "br0"}})
	if r[0].Stations[0].NeighborIPv4 != "" {
		t.Fatal("matched across an unverified bridge")
	}
}

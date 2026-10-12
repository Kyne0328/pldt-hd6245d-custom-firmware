package device

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOptionalLeaseNamesRequireThreeIndependentMatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dnsmasq.leases")
	now := time.Unix(1_800_000_000, 0)
	text := "1900000000 02:11:22:33:44:55 192.168.1.17 laptop *\n" +
		"1900000000 02:11:22:33:44:66 192.168.1.18 other-laptop *\n" +
		"1700000000 02:11:22:33:44:77 192.168.1.19 expired *\n" +
		"1900000000 02:11:22:33:44:88 192.168.1.21 unsafe/name *\n"
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	radios := []WirelessRadio{{Interface: "wl0", Master: "br0", AssociationStatus: "observed", Stations: []WirelessStation{
		{MAC: "02:11:22:33:44:55", NeighborIPv4: "192.168.1.17"},
		{MAC: "02:11:22:33:44:66", NeighborIPv4: "192.168.1.200"}, // not same IP
		{MAC: "02:11:22:33:44:77", NeighborIPv4: "192.168.1.19"},  // expired
		{MAC: "02:11:22:33:44:88", NeighborIPv4: "192.168.1.21"},  // invalid name
	}}}
	result := InspectDHCPLeaseEvidence(radios, path, now)
	if !result.Available || result.ValidRows != 3 || result.MatchedWirelessNames != 1 || radios[0].Stations[0].LeaseHostname != "laptop" {
		t.Fatalf("wrong evidence %+v / %+v", result, radios)
	}
	for _, st := range radios[0].Stations[1:] {
		if st.LeaseHostname != "" {
			t.Fatal("unverified hostname surfaced")
		}
	}
	if err := os.Symlink(path, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	rejected := InspectDHCPLeaseEvidence(radios, filepath.Join(dir, "link"), now)
	if rejected.Available {
		t.Fatal("followed a symbolic link to lease file")
	}
	absent := InspectDHCPLeaseEvidence(radios, path+".missing", now)
	if absent.Available {
		t.Fatal("invented lease source")
	}
}
func TestDuplicateLeaseNameWithheld(t *testing.T) {
	r := []WirelessRadio{{Interface: "wl0", AssociationStatus: "observed", Stations: []WirelessStation{{MAC: "02:11:22:33:44:55", NeighborIPv4: "192.168.1.17"}}}}
	lease := observedLease{MAC: "02:11:22:33:44:55", IP: "192.168.1.17", Name: "laptop"}
	if AttachLeaseNames(r, []observedLease{lease, lease}) != 0 || r[0].Stations[0].LeaseHostname != "" {
		t.Fatal("accepted duplicate lease")
	}
}

package admin

import (
	"bytes"
	"testing"
)

func TestStockMenuMappedIntoIndependentConsole(t *testing.T) {
	views := []string{"overview", "status", "network", "security", "applications", "management", "storage", "services"}
	for _, name := range views {
		if !bytes.Contains(indexHTML, []byte(`data-view="`+name+`"`)) {
			t.Fatalf("missing navigation group %q", name)
		}
	}
	sections := []string{"catalog-status", "catalog-network", "catalog-security", "catalog-applications", "catalog-management", "lan-ports"}
	for _, id := range sections {
		if !bytes.Contains(indexHTML, []byte(`id="`+id+`"`)) {
			t.Fatalf("missing console section %q", id)
		}
	}
	labels := []string{"Wi-Fi clients list", "Read-only driver probe", "wirelessClientFacts", "Kernel CONFIG_TUN", "WireGuard feasibility", "VPN pass-through (GRE)", "IPsec pass-through", "WireGuard VPN", "OpenVPN", "Dynamic DNS", "Firmware update", "Port forwarding", "Parental controls", "VoIP status", "Optical status", "DHCP client list"}
	for _, label := range labels {
		if !bytes.Contains(appJS, []byte(label)) {
			t.Fatalf("missing feature %q", label)
		}
	}
	if bytes.Contains(indexHTML, []byte("/goform/")) || bytes.Contains(appJS, []byte(`fetch("/goform/`)) {
		t.Fatal("independent HGFW UI must never submit vendor configuration requests")
	}
	if bytes.Contains(appJS, []byte("innerHTML")) {
		t.Fatal("unsafe untrusted HTML insertion")
	}
	if !bytes.Contains(appJS, []byte("Not enabled")) || !bytes.Contains(appJS, []byte("Not configured or established by HGFW")) {
		t.Fatal("feature capability states must be explicit")
	}
	for _, label := range []string{"serviceSources", "dnsUDP53Observed", "dhcpUDP67Observed", "HGFW read-only kernel/file/source checks"} {
		if !bytes.Contains(appJS, []byte(label)) {
			t.Fatalf("missing evidence-driven source status: %s", label)
		}
	}
}

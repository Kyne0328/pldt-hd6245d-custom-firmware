package device

import (
	"bufio"
	"net/netip"
	"os"
	"strings"
)

// ResolverInspection is a bounded observation of Linux's own resolver file.
// It is not a DNS server, DHCP advertisement, or upstream reachability test.
type ResolverInspection struct {
	Available   bool     `json:"available"`
	Nameservers []string `json:"nameservers"`
	Source      string   `json:"source"`
	Notes       []string `json:"notes"`
}

func InspectResolvers(path string) ResolverInspection {
	out := ResolverInspection{Nameservers: []string{}, Source: "Linux resolver configuration", Notes: []string{
		"These are OS resolver addresses, not proof of working DNS or client DHCP settings.",
		"No DNS query or network change is performed.",
	}}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	out.Available = true
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 256), 2048)
	seen := map[string]bool{}
	for i := 0; i < 128 && scan.Scan(); i++ {
		raw := strings.TrimSpace(scan.Text())
		if strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, ";") {
			continue
		}
		fields := strings.Fields(raw)
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}
		addr, e := netip.ParseAddr(fields[1])
		if e != nil || addr.Zone() != "" || addr.IsUnspecified() || addr.IsMulticast() {
			continue
		}
		name := addr.String()
		if !seen[name] && len(out.Nameservers) < 4 {
			out.Nameservers = append(out.Nameservers, name)
			seen[name] = true
		}
	}
	return out
}

package device

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// SourceProbe reports only whether an explicitly named, common configuration
// or lease-file location is present. Contents, link targets and device
// identifiers are deliberately never read or included in responses.
type SourceProbe struct {
	Label   string `json:"label"`
	Present bool   `json:"present"`
	Kind    string `json:"kind"`
}

type ServiceDiscovery struct {
	ResolverCandidates         []SourceProbe `json:"resolverCandidates"`
	LeaseCandidates            []SourceProbe `json:"leaseCandidates"`
	VendorDHCPConfigCandidates []SourceProbe `json:"vendorDHCPConfigCandidates"`
	KnownProcessNames          []string      `json:"knownProcessNames"`
	DNSUDP53Observed           bool          `json:"dnsUDP53Observed"`
	DNSTCP53Listening          bool          `json:"dnsTCP53Listening"`
	DHCPUDP67Observed          bool          `json:"dhcpUDP67Observed"`
	Notes                      []string      `json:"notes"`
}

var resolverCandidatePaths = []string{
	"/etc/resolv.conf", "/tmp/resolv.conf", "/var/run/resolv.conf", "/var/etc/resolv.conf",
}
var leaseCandidatePaths = []string{
	"/var/lib/misc/dnsmasq.leases", "/var/run/dnsmasq.leases",
	"/tmp/dnsmasq.leases", "/tmp/dhcp.leases", "/var/udhcpd.leases", "/var/run/udhcpd.leases",
	// Confirmed as a compiled-in path in the owner's original RP2613 udhcpd ELF.
	"/var/udhcpd/udhcpd.leases",
}
var vendorDHCPConfigPaths = []string{
	"/fhconf/udhcpd.conf", "/fhconf/udhcpdbinding.conf",
}
var knownServiceComm = map[string]bool{
	"dnsmasq": true, "udhcpd": true, "dhcpd": true,
	"odhcpd": true, "dnsd": true, "dnrd": true, "named": true, "dnsproxy": true,
	"dnsrelay": true, "dhcpl2": true,
}

func probeStaticPaths(root string, paths []string) []SourceProbe {
	items := make([]SourceProbe, 0, len(paths))
	for _, label := range paths {
		item := SourceProbe{Label: label, Kind: "absent"}
		file := filepath.Join(root, strings.TrimPrefix(label, "/"))
		st, err := os.Lstat(file)
		if err == nil {
			item.Present = true
			switch {
			case st.Mode().IsRegular():
				item.Kind = "regular file"
			case st.Mode()&os.ModeSymlink != 0:
				item.Kind = "symlink (target not inspected)"
			case st.IsDir():
				item.Kind = "directory"
			default:
				item.Kind = "other"
			}
		}
		items = append(items, item)
	}
	return items
}

func processNameEvidence(procPath string) []string {
	entries, err := os.ReadDir(procPath)
	if err != nil {
		return []string{}
	}
	found := map[string]bool{}
	for i, ent := range entries {
		if i >= 2048 {
			break
		}
		if !ent.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(ent.Name()); err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(procPath, ent.Name(), "comm"))
		if err != nil || len(raw) > 64 {
			continue
		}
		name := strings.TrimSpace(string(raw))
		if knownServiceComm[name] {
			found[name] = true
		}
	}
	result := make([]string, 0, len(found))
	for name := range found {
		result = append(result, name)
	}
	// Deterministic order prevents leaking iteration order or PID information.
	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && result[j] < result[j-1]; j-- {
			result[j], result[j-1] = result[j-1], result[j]
		}
	}
	return result
}

func observedPort(procFile string, port uint16, tcp bool) bool {
	f, err := os.Open(procFile)
	if err != nil {
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256), 4096)
	if !scanner.Scan() {
		return false
	}
	for i := 0; i < 2048 && scanner.Scan(); i++ {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		p := strings.Split(fields[1], ":")
		if len(p) != 2 {
			continue
		}
		n, e := strconv.ParseUint(p[1], 16, 16)
		if e != nil || n != uint64(port) {
			continue
		}
		if tcp && fields[3] != "0A" {
			continue
		} // TCP LISTEN only
		return true
	}
	return false
}

// DiscoverServiceEvidence checks only fixed, well-known paths, comm process
// names from an allowlist and port numbers in procfs. It does not open
// proprietary configuration, read cmdline/environ or make any DNS requests.
func DiscoverServiceEvidence(root string) ServiceDiscovery {
	proc := filepath.Join(root, "proc")
	result := ServiceDiscovery{
		ResolverCandidates:         probeStaticPaths(root, resolverCandidatePaths),
		LeaseCandidates:            probeStaticPaths(root, leaseCandidatePaths),
		VendorDHCPConfigCandidates: probeStaticPaths(root, vendorDHCPConfigPaths),
		KnownProcessNames:          processNameEvidence(proc),
		Notes: []string{
			"Vendor DHCP file paths originate from offline RP2613 udhcpd ELF strings, not observed live service use; no file content is read.",
			"Candidate path presence does not establish file format or an active DHCP/DNS service.",
			"Process names and UDP/TCP socket ports do not establish ownership or reachability.",
			"No private file contents, client identities, CLI arguments or configurations are inspected.",
		},
	}
	result.DNSUDP53Observed = observedPort(filepath.Join(proc, "net", "udp"), 53, false) ||
		observedPort(filepath.Join(proc, "net", "udp6"), 53, false)
	result.DNSTCP53Listening = observedPort(filepath.Join(proc, "net", "tcp"), 53, true) ||
		observedPort(filepath.Join(proc, "net", "tcp6"), 53, true)
	result.DHCPUDP67Observed = observedPort(filepath.Join(proc, "net", "udp"), 67, false) ||
		observedPort(filepath.Join(proc, "net", "udp6"), 67, false)
	return result
}

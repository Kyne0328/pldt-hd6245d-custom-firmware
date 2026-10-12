package device

import (
	"bufio"
	"io"
	"net/netip"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DHCPLeaseEvidence reflects *only* an optional dnsmasq-format lease file.
// It does not establish that the stock DHCP service is dnsmasq, nor expose
// all LAN clients or private hostname data in the API.
type DHCPLeaseEvidence struct {
	Available            bool   `json:"available"`
	Format               string `json:"format"`
	ValidRows            int    `json:"validRows"`
	MatchedWirelessNames int    `json:"matchedWirelessNames"`
}
type observedLease struct{ MAC, IP, Name string }

var safeLeaseName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)

// readDNSMasqLeases reads at most 64 KiB from an ordinary (non-symlink) file.
// It intentionally avoids shell commands, vendor proprietary configuration,
// lease client IDs and expired records.
func readDNSMasqLeases(path string, now time.Time) ([]observedLease, bool) {
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() > 65536 {
		return nil, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	scanner := bufio.NewScanner(io.LimitReader(f, 65537))
	scanner.Buffer(make([]byte, 1024), 4096)
	out := []observedLease{}
	for scanner.Scan() {
		if len(out) >= 128 {
			break
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) != 5 {
			continue
		}
		expire, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil || (expire != 0 && expire <= now.Unix()) {
			continue
		}
		mac := normalMAC(fields[1])
		ip, err := netip.ParseAddr(fields[2])
		if mac == "" || err != nil || !ip.Is4() || ip.IsUnspecified() || ip.IsMulticast() {
			continue
		}
		name := fields[3]
		if !safeLeaseName.MatchString(name) || name == "*" {
			name = ""
		}
		out = append(out, observedLease{MAC: mac, IP: ip.String(), Name: name})
	}
	if scanner.Err() != nil {
		return nil, false
	}
	return out, true
}

// AttachLeaseNames does not create a client or IP. It attaches a DNSMasq-format
// lease host label only if MAC and IP agree with an already associated station
// AND a separate kernel ARP match. Duplicate/conflicting matches are discarded.
func AttachLeaseNames(radios []WirelessRadio, leases []observedLease) int {
	found := 0
	for ri := range radios {
		if radios[ri].AssociationStatus != "observed" {
			continue
		}
		for si := range radios[ri].Stations {
			st := &radios[ri].Stations[si]
			if st.NeighborIPv4 == "" {
				continue
			}
			count := 0
			var candidate string
			for _, lease := range leases {
				if lease.MAC == normalMAC(st.MAC) && lease.IP == st.NeighborIPv4 {
					count++
					candidate = lease.Name
				}
			}
			if count == 1 && candidate != "" {
				st.LeaseHostname = candidate
				found++
			}
		}
	}
	return found
}

func InspectDHCPLeaseEvidence(radios []WirelessRadio, path string, now time.Time) DHCPLeaseEvidence {
	result := DHCPLeaseEvidence{Format: "dnsmasq (optional; not assumed stock service)"}
	leases, found := readDNSMasqLeases(path, now)
	result.Available = found
	result.ValidRows = len(leases)
	if found {
		result.MatchedWirelessNames = AttachLeaseNames(radios, leases)
	}
	return result
}

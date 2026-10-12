package device

import (
	"bufio"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// VPNKernelEvidence is a non-mutating kernel capability assessment. It never
// opens /dev/net/tun, calls TUNSETIFF or loads a kernel module.
type VPNKernelEvidence struct {
	TUNRegistered         *bool  `json:"tunRegistered,omitempty"`
	TUNKernelConfig       string `json:"tunKernelConfig"`
	WireGuardKernelConfig string `json:"wireGuardKernelConfig"`
	XFRMStatPresent       bool   `json:"xfrmStatPresent"`
	TunAssessment         string `json:"tunAssessment"`
	WireGuardAssessment   string `json:"wireGuardAssessment"`
}

func inspectKernelConfig(path string) (tun, wg string) {
	tun, wg = "unknown", "unknown"
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	var rd io.Reader = file
	if strings.HasSuffix(path, ".gz") {
		gz, e := gzip.NewReader(file)
		if e != nil {
			return
		}
		defer gz.Close()
		rd = gz
	}
	// The extracted status only includes two non-secret Kconfig booleans.
	scan := bufio.NewScanner(io.LimitReader(rd, 2<<20))
	scan.Buffer(make([]byte, 1024), 8192)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		for _, entry := range []struct {
			key string
			out *string
		}{{"CONFIG_TUN", &tun}, {"CONFIG_WIREGUARD", &wg}} {
			if line == entry.key+"=y" {
				*entry.out = "built-in"
			}
			if line == entry.key+"=m" {
				*entry.out = "module"
			}
			if line == "# "+entry.key+" is not set" {
				*entry.out = "disabled"
			}
		}
	}
	return
}
func InspectVPNKernel(root string, devicePresent, modulePresent, wireguardPresent bool) VPNKernelEvidence {
	p := VPNKernelEvidence{TUNKernelConfig: "unknown", WireGuardKernelConfig: "unknown"}
	lines := boundedLines(filepath.Join(root, "proc/misc"), 512)
	if len(lines) > 0 {
		t := false
		for _, l := range lines {
			f := strings.Fields(l)
			if len(f) == 2 && f[1] == "tun" {
				t = true
			}
		}
		p.TUNRegistered = &t
	}
	tun, wg := inspectKernelConfig(filepath.Join(root, "proc/config.gz"))
	p.TUNKernelConfig, p.WireGuardKernelConfig = tun, wg
	if tun == "unknown" {
		if cfg, err := filepath.Glob(filepath.Join(root, "boot", "config-*")); err == nil && len(cfg) > 0 {
			tun, wg = inspectKernelConfig(cfg[0])
			p.TUNKernelConfig, p.WireGuardKernelConfig = tun, wg
		}
	}
	if _, err := os.Stat(filepath.Join(root, "proc/net/xfrm_stat")); err == nil {
		p.XFRMStatPresent = true
	}
	switch {
	case devicePresent && p.TUNRegistered != nil && *p.TUNRegistered:
		p.TunAssessment = "TUN character device and kernel registration observed; no ioctl, tunnel or credentials tested"
	case devicePresent:
		p.TunAssessment = "TUN device node found, but active kernel registration unconfirmed"
	case modulePresent || p.TUNRegistered != nil && *p.TUNRegistered || p.TUNKernelConfig == "built-in":
		p.TunAssessment = "TUN kernel evidence found, but usable /dev/net/tun was not verified"
	case p.TUNKernelConfig == "disabled":
		p.TunAssessment = "Kernel config disables TUN; userspace TUN VPN requires a compatible new kernel"
	default:
		p.TunAssessment = "TUN support not demonstrated; a missing node alone is inconclusive"
	}
	switch {
	case wireguardPresent:
		p.WireGuardAssessment = "WireGuard module loaded or built in; interfaces and connectivity untested"
	case p.WireGuardKernelConfig == "built-in":
		p.WireGuardAssessment = "WireGuard compiled in according to kernel config; runtime support untested"
	case p.WireGuardKernelConfig == "module":
		p.WireGuardAssessment = "WireGuard configured as module; module presence and compatibility unverified"
	case p.WireGuardKernelConfig == "disabled":
		p.WireGuardAssessment = "Kernel config disables in-kernel WireGuard; userspace alternative would require validated TUN"
	default:
		p.WireGuardAssessment = "No in-kernel WireGuard support observed; userspace path unverified"
	}
	return p
}

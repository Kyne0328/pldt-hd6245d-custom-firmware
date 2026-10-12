# HGFW — HG6245D Custom Firmware

[![Build and test](https://github.com/Kyne0328/pldt-hd6245d-custom-firmware/actions/workflows/ci.yml/badge.svg)](https://github.com/Kyne0328/pldt-hd6245d-custom-firmware/actions/workflows/ci.yml)

**An open, community-oriented firmware project to repurpose the FiberHome HG6245D as a capable router and network appliance.**

The aim is to use as much of the device's existing hardware as possible: dual-band Wi-Fi, Ethernet routing, USB storage, printer sharing, and an independent administration interface.

> [!IMPORTANT]
> **Developer preview — not ready to install.** The project now builds a native ARMv7 management daemon, an authenticated browser dashboard and a reproducible userspace overlay. However, it is **not** a complete router operating system, bootable kernel, or flashable image. No generated archive should be flashed to a device.

## The goal

```mermaid
flowchart TB
    internet(("Internet"))
    main["Existing main router"]
    hg["HG6245D running future HGFW"]
    devices["Phones · laptops · IoT"]
    drive[("USB drive")]
    printer["USB printer"]
    lan["Ethernet clients"]

    internet --> main
    main -. "5 GHz wireless uplink, or Ethernet WAN" .-> hg
    hg -->|"2.4 GHz / 5 GHz AP"| devices
    hg -->|"LAN ports"| lan
    drive <-->|"USB 2.0"| hg
    printer <-->|"USB 2.0"| hg

    classDef hardware fill:#e8f2ff,stroke:#3871ba,color:#133c6b;
    classDef client fill:#edf9f0,stroke:#478c5b,color:#173c22;
    class hg hardware;
    class devices,drive,printer,lan client;
```

This diagram is the **planned system**, not an assertion that the current code already implements these connections. GPON operation is optional because the project targets retired/deactivated units.

### Target hardware

| Component | Findings |
| --- | --- |
| Device | FiberHome HG6245D, RP2613 hardware/firmware variant |
| SoC | Broadcom BCM96846 (`brcm,bcm96846` device-tree compatible) |
| CPU / OS baseline | ARMv7, original Broadcom Linux 4.1.45 |
| RAM | Approximately 208 MiB visible to the stock Linux kernel |
| Wi-Fi | 2.4 GHz and 5 GHz Broadcom radios; AP mode verified, client capability advertised but not yet tested end-to-end |
| Ethernet | Four LAN ports; switch and per-port routing still to be validated |
| USB | Two physical ports verified to enumerate a removable USB mass-storage device at USB 2.0 high-speed |
| Flash | NAND / UBI / UBIFS with two firmware slots; bootloader and safe recovery behavior not yet established |

Hardware capabilities verified on one board are not necessarily identical across all ISP/manufacturing variants.

## Progress

| Area | Status |
| --- | --- |
| Original firmware preservation / hardware investigation | Completed locally; private backups **not included** here |
| ARMv7 static cross-compilation | Working |
| Service supervisor (explicit configurations, bounded restarts and shutdown) | Implemented; host-tested |
| Password-protected local dashboard and live device status | **Verified on physical BCM96846:** authenticated loopback HTTP/API (`SMOKE_EXIT=0`), plus real-browser HTTPS login and dashboard on LAN (`PREVIEW_EXIT=0`, timed shutdown, Oct 12, 2026). |
| USB device detection and opt-in read-only mount/unmount CLI | Implemented; hardware access not yet tested with HGFW |
| Network Engine v1 (read-only) | **Verified on physical BCM96846:** kernel IPv4 route/bridge/link/traffic inventory (`INSPECT_EXIT=0`); loopback health responder launched, checked and cleanly stopped by process supervisor (`SELFTEST_EXIT=0`), October 12, 2026. Network configuration changes not tested or enabled. |
| Reproducible userspace overlay + checksums | Working |
| Offline Python and Go test suites | Working |
| Native execution on the HG6245D | **Stage 2 softfloat ARMv7 binary verified on physical BCM96846**: `version`, `status`, and `usb` each returned exit code 0 from RAM-backed tmpfs (Oct 12, 2026). Initial hardfloat build was incompatible; `GOARM=7,softfloat` fixed it. |
| Ethernet WAN, firewall, DHCP and DNS | Read-only diagnostics implemented; independent DHCP/DNS/firewall/routing changes pending. |
| Wireless client / repeater modes | **Verified on physical BCM96846 (Oct 12, 2026):** read-only Broadcom `wl assoclist` returned associated-station inventories for both `wl0` and `wl1` (`WIRELESS_EXIT=0`); 1 station on 2.4 GHz, 2 on 5 GHz in one snapshot. SSID parsing needs separate validation; AP/STA/repeater changes remain unsupported. |
| SMB/NAS file sharing | Planned |
| USB printer server | Planned; requires printer compatibility testing |
| Independent admin UI | Eight-view router console: Overview, Status, Network, Security, Applications/VPN, Management, USB Storage, and Services; live kernel diagnostics plus explicit unimplemented states. Host browser tested; latest hardware retest pending. |
| Bootable custom image / safe installer | Blocked by boot-chain and recovery validation |

### Loopback dashboard smoke test

Run `hgfw smoke` with the native host executable or the ARMv7 executable. It starts the real admin HTTP handler on an **OS-assigned loopback-only TCP port**, uses a temporary random password held only in memory, and checks anonymous access denial, login, protected dashboard and JSON API, CSRF enforcement, logout revocation, server shutdown, and process memory usage. It writes no credentials, makes no router configuration changes, and prints a JSON summary without passwords or device MAC addresses.

```sh
./build/hgfw-host smoke
# For an HG6245D, run the verified ARM softfloat binary from tmpfs:
# /mnt/hgfw-run/hgfw smoke
```

The full end-to-end smoke test passed on **both the no-VFP ARM emulator and a physical HG6245D** running stock Broadcom Linux 4.1.45 (October 12, 2026). The physical test returned `SMOKE_EXIT=0`, all seven security/lifecycle checks true, 37 interfaces, two USB block-device entries (one disk and one partition), 12,020 KiB peak RSS, and 9,647 ms elapsed. It used `127.0.0.1:ephemeral` and automatically shut down. This establishes backend behavior, not browser rendering, long-running reliability, full USB filesystem support, router/network configuration control, or an independently bootable firmware. Because it binds to the router's 127.0.0.1, it is intentionally not browsable from another computer on the LAN.

### Temporary HTTPS browser preview (engineering validation)

To view the real HGFW dashboard from another LAN computer, stage the ARMv7 executable on the router using a RAM-backed filesystem and a SHA-256 checked transfer. The source includes `hgfw preview`, which **never modifies network configuration** and does not install any boot service. With the router's example LAN management address:

```sh
/mnt/hgfw-run/hgfw preview --listen 192.168.1.2:9443 --seconds 240
```

The preview binds only to an IPv4 address already assigned to the device; wildcard binds are refused. It creates a fresh TLS key/certificate and strong random password **in memory**, prints the temporary `https://` URL, password and certificate SHA-256 fingerprint to the terminal, and shuts down automatically after 240 seconds (or sooner on Ctrl-C). HTTPS clients will warn because the certificate is self-signed. Verify the certificate fingerprint independently against the terminal before proceeding; do not permanently trust the certificate. Never share the printed one-time password or commit credentials to Git.

**Physical-device browser test verified (October 12, 2026):** The operator successfully opened the HTTPS login and dashboard from a Windows browser with the preview running on the stock HG6245D. The router returned `HGFW_PREVIEW_STOPPED=true`, `HGFW_PREVIEW_ELAPSED_SECONDS=244`, `HGFW_PREVIEW_PEAK_RSS=12500 kB` (about 12.2 MiB), and `PREVIEW_EXIT=0`. This verifies actual browser access and timed process exit, not long-term stability or independence from the vendor OS. The dashboard is observation-only. The command does not run router services, DHCP, firewall configuration, flash utilities or USB mounts. Afterward it prints elapsed runtime and its peak resident memory reported by the kernel. This is a bounded LAN demonstration of the **actual** management UI, not a deployed router OS. Do not expose the preview port through a WAN or port-forwarding rule.

### Verified non-flashing hardware test

On a physical HG6245D running stock RP2613 Linux 4.1.45, the independently built HGFW Stage 2 ARMv7 binary was SHA-256 verified, copied to a temporary RAM-backed filesystem, and executed without a firmware change. The `version`, `status`, and `usb` commands all completed with exit code 0. `status` identified `Broadcom BCM96846` and enumerated stock Ethernet, switch and dual-band Wi-Fi interfaces; `usb` enumerated a removable disk and partition. The binary and temporary mount were removed afterward.

**Scope:** These were read-only device-inventory commands. Separate follow-up tests successfully verified the authenticated dashboard on physical hardware, but did not verify USB filesystem mounting, process supervision, router control, a new-kernel boot, or flash recovery. Device-specific network MAC addresses, passwords and raw console transcripts are intentionally not published.

## Architecture and development path

```mermaid
flowchart LR
    code["HGFW source"] --> build["Clang + Go + Python"]
    build --> overlay["ARMv7 daemon and userspace overlay"]
    overlay --> tests["Archive verification + tests"]
    tests --> future["Future non-flashing device tests"]
    future -. "Only after validated recovery" .-> release["Future bootable firmware"]

    stock["Optional owner-supplied stock BusyBox"] -. "Local compatibility build only" .-> compat["Experimental rootfs"]
    overlay -.-> compat

    classDef done fill:#e6f6eb,stroke:#3f8e58,color:#16462a;
    classDef future fill:#fff4dc,stroke:#bf8a2e,color:#65460f;
    class code,build,overlay,tests done;
    class future,release,compat future;
```

A complete modern Linux/OpenWrt-style port is the long-term direction. Broadcom's proprietary wireless/switching dependencies make modern-kernel support a substantial engineering challenge. A compatibility userspace on the original kernel is a development stepping stone—not a secure, finished firmware release.

## Build

**Requirements:** Linux or WSL2, Go 1.26+, Python 3.9+, GNU Make, Bash, Clang with ARM target support, and LLD. Install Go from [go.dev/dl](https://go.dev/dl/) or an equivalent trusted package source. On Ubuntu 24.04, install the remaining tools:

```bash
sudo apt-get install -y python3 make clang lld
# Ensure `go version` reports Go 1.26 or newer.
git clone https://github.com/Kyne0328/pldt-hd6245d-custom-firmware.git
cd pldt-hd6245d-custom-firmware
make all
```

`make all` cross-builds both the original ARM canary and a statically linked ARMv7 HGFW management daemon using **`GOARM=7,softfloat`**, assembles the overlay, verifies integrity and runs the Python and Go tests. **The build does not contact or modify any router.**

The outputs are generated under `dist/`:

- `hgfw-stage2-armv7-overlay.tar.gz` — nonbootable ARMv7 userspace overlay containing the HGFW daemon, canary, default configuration and read-only diagnostics.
- `manifest.json` and `*.sha256` — build metadata and integrity checks.

### HGFW console redesign

The management interface is now a proper responsive, four-view router console rather than one long inventory page. **Overview** shows a local management/network diagram, measured RAM utilization, uptime, load, wireless radio presence, USB disk counts and key device summaries. **Network** has searchable, filterable Broadcom/Linux interfaces with state, IP, MAC and MTU; **USB storage** distinguishes disks, partitions and actual mounts; **Services** shows the supervisor's live process states or a truthful empty state. Views work on mobile and desktop, refresh live every 15 seconds, and remain accessible without a third-party font, JS library or internet connection. All content is rendered from the authenticated read-only status API. Interface presence is not presented as internet connectivity or proof of control over drivers. A visual browser test of the redesigned version on the physical router is still pending.

### Wireless Engine v1 and VPN kernel assessment

`hgfw wireless-inspect` and the authenticated **Status -> Wi-Fi clients / 2.4 GHz / 5 GHz** pages now attempt *strictly read-only* native association queries: the Broadcom `wl -i wlN assoclist` driver helper when present, or `iw dev wlN station dump` for compatible nl80211 drivers. Radio SSID and channel are read through permitted `wl ... ssid/channel` or `iw ... info` queries only when the helper responds. Each command has a 900 ms timeout, an output bound and fixed arguments. Neither helper is bundled in HGFW. The extracted original application partition did **not** include a standalone `wl` or `iw` executable, although its proprietary `webs` webserver references a native Wi-Fi station enumeration routine; the *running* HG6245D **does** expose a working `wl` helper for the strictly read-only `assoclist` query, as confirmed by the October 12 physical test. A missing/unsupported driver query is reported as **unavailable**, not as zero connected stations. Never interpret IPv4 ARP neighbors as authenticated Wi-Fi associations. Associated MAC addresses are shown only in the authenticated dashboard/explicit local CLI output; do not post raw client lists publicly.

`hgfw vpn-inspect` and **Applications -> VPN overview** distinguish the TUN character node, kernel `/proc/misc` TUN registration, available module/sysfs evidence, optional `/proc/config.gz` or `/boot/config-*` `CONFIG_TUN`/`CONFIG_WIREGUARD` flags and presence of `/proc/net/xfrm_stat`. A bounded kernel-config reader extracts just these booleans. No TUN device is opened, no `TUNSETIFF` ioctl is invoked and no kernel modules or routes are changed. This is **VPN feasibility evidence**, not a working VPN tunnel, keys or authenticated server/client.

Read-only commands for a RAM-only HG6245D hardware test (the JSON can contain Wi-Fi MACs and an SSID, so redact before sharing):

```sh
/mnt/hgfw-run/hgfw wireless-inspect
/mnt/hgfw-run/hgfw vpn-inspect
```

**Physical test (October 12, 2026):** after hash-verifying ARMv7 softfloat binary SHA-256 `6a4d56cc3ba90b0dfdad9f3b62c9585625c8ac137e18dce468bb806dd0d5b246` in a 12 MiB dedicated tmpfs, `wireless-inspect` returned `WIRELESS_EXIT=0`, `associationStatus=observed` for both `wl0` (1 station) and `wl1` (2 stations). The tool reported channels 9 and 58. Parsed SSID was only one character (`I`) on both radios; this is **unverified** and should not be treated as a confirmed SSID until the raw command format can be safely inspected. No RSSI/rate measurements were obtained from `assoclist`. `vpn-inspect` returned `VPN_EXIT=0` and observed **no TUN character device**, **no TUN registration**, and reported `CONFIG_TUN=disabled`, with `CONFIG_WIREGUARD=unknown`, no WireGuard module observed and only `pppd` detected. IPv4 forwarding was off, IPv6 forwarding on. This means the stock kernel does not currently provide a demonstrated TUN-based path for userspace WireGuard/OpenVPN. No station MACs or private SSIDs are included in this public record. Actual VPN deployment requires different kernel/driver support or a separately validated alternative, along with secure key storage, routing and verified recovery. No flash or current network configuration changes occur.
### Full router-console feature map (Stage 2 read-only)

HGFW now uses the **five original PLDT administration areas** as an independently implemented navigation framework: **Status**, **Network**, **Security**, **Applications**, and **Management**. Existing **Overview**, **USB Storage**, and **Services** remain available. This structure was derived from offline analysis of the private stock menu XML and matching ASP entry points, **not** from copying vendor JavaScript or compiled binaries into HGFW. The original Wi-Fi client table was traced to `wireless/wifimaclist.asp` and a compiled `getWifiMac_IPListParam` handler. Stock GRE and IPsec pass-through pages submit to private `/goform/setVPNEnable` and `/goform/setIpsecEnable`; neither page proves VPN server availability.

- **Live, authenticated observations:** device model, kernel, memory, uptime, Linux/bridge/WAN route evidence, available carrier readings, an operator-reported LAN 1-4 map on BCM96846 only, completed ARP neighbor entries, USB disks, HGFW process supervisor, kernel IPv4/IPv6 forwarding flags, available firewall table names, VPN executable inventory, and TUN/WireGuard kernel/device presence.
- **Features not yet independently enabled:** Wi-Fi association/client list (ARP is not a Wi-Fi list), SSID/WPA/5 GHz radio configuration, WAN/DHCP/IPTV/IPv6 configuration, VPN pass-through toggles, WireGuard/OpenVPN tunnels, firewall/parental filtering, NAT/port forwarding/UPnP/DDNS, SMB/printing, account/password changes, logging/clock, backups/restores, firmware update and reboot. They have navigable explanations and prerequisites, **not working Apply buttons**.
- **Safety boundary:** The dashboard neither submits stock CGI calls nor executes router configuration/flash commands. VPN diagnostics are capability signals, **not an active VPN connection**. Original stock source and private backups remain outside the public repository.

This milestone implements the **router-administration information architecture with safe diagnostics**, not functional parity with stock firmware. Each configuration service still needs independent driver/backend integration, tests and a proven software-only rollback/recovery mechanism. The latest version's physical-device browser test is pending; earlier HGFW builds have been verified on hardware.
### Network Engine v1 — inspection, validation, supervised loopback

The Stage 2 backend now supplies a `device.network` object in authenticated `/api/status` and `hgfw status`. Its kernel evidence comes from `/sys/class/net` and `/proc/net/route`, including interface role labels with explicit **confidence**, bridge master (when exposed), operative state, carrier (when available), link speed (when available), RX/TX counters and IPv4 routes. The Network page displays these observed values and highlights the active default IPv4 route where one exists. The engine **does not map physical LAN jack numbers** and does **not verify** Wi-Fi AP/STA mode or actual internet access; vendor-named `wan0` and GPON interfaces are not assumed functional WAN ports.

```sh
hgfw network-inspect
hgfw network-plan --config config/network.example.json
hgfw network-plan --config config/network.example.json --check-device
hgfw network-selftest
```

`network-plan` validates interface-name safety, non-optical interface choices, LAN CIDR and a DHCP range excluding the router IP, subnet address and broadcast address. `--check-device` additionally requires the selected interface names to exist in the running Linux inventory and will reject the example plan on devices where those hypothetical names do not exist. **No plan is applied.** These checks do not prove routing, firewall or hardware driver feasibility.

`network-selftest` starts a dedicated temporary health responder on an OS-selected `127.0.0.1` port through HGFW's existing independent process supervisor, checks the loopback HTTP response and waits for clean termination. The command produces JSON (`success`, `responseVerified`, `supervised`, `stopped`). This is *loopback-bound process isolation*, not a new Linux network namespace or a router DHCP/DNS service. It never changes interfaces, bridges, routes, DNS, WAN, wireless or flash, and the child has a 25-second upper time limit. **Physical BCM96846 validation (October 12, 2026):** After transfer to a 12 MiB RAM-only tmpfs and SHA-256 verification, `network-inspect` returned `INSPECT_EXIT=0`. The kernel identified `br0` as a Linux bridge; `eth0.0`, `eth1.0`, `eth2.0`, `eth3.0`, `wl0` and `wl1` were among its members. `eth0` and `eth3` reported carrier; `eth1` and `eth2` reported no carrier. Only one local IPv4 route, `192.168.1.0/24` via `br0`, appeared; no default IPv4 route was observed. The separate `network-selftest` returned `SELFTEST_EXIT=0`, with `success`, `responseVerified`, `supervised` and `stopped` all true (112 ms). RAM test artifacts were subsequently removed. This confirms read-only hardware inspection and a local supervised service, **not** the physical jack map, WAN connectivity, independent routing/DHCP/DNS, or a safe flash/recovery path. No device MAC addresses, private credentials or raw console dumps are published.

## Run the dashboard locally

First, build a **host** executable. Never run host binaries on the router.

```bash
make host
mkdir -p private
read -rsp "Choose a strong admin password: " HGFW_PASS; echo
(umask 077; printf '%s' "$HGFW_PASS" > private/admin-password)
unset HGFW_PASS
./build/hgfw-host init --password-file private/admin-password --auth-file private/admin.json
rm private/admin-password
./build/hgfw-host serve --listen 127.0.0.1:9080 --auth-file private/admin.json --services config/services.json
```

Open **http://127.0.0.1:9080** on the same development machine. The login has password-based authentication, an ephemeral session, a CSRF-protected sign-out and rate limiting. The dashboard shows live kernel network, RAM, and USB information from the machine **where the daemon actually runs**.

For a non-loopback listener, a TLS certificate and key must be configured; default HTTP is intentionally local-only. The web interface exposes **read-only** state and provides no NAND, Wi-Fi, or firewall write endpoints.

The CLI provides independent system inspection and an isolated LAN/WAN **plan** without applying it:

```bash
./build/hgfw-host status
./build/hgfw-host usb
./build/hgfw-host network-plan --config config/network.example.json
```

The service supervisor uses `config/services.json`, with an empty and disabled-by-default service list. Operators can explicitly add absolute executable paths and restart policies. External child processes run without a shell and are terminated when HGFW stops. USB mounting is available **only** through an explicit local CLI action that verifies a sysfs USB partition and mounts it read-only under a pre-provisioned directory; do not attempt that on the router until its automount behavior has been assessed.

Optional developer-only compatibility rootfs:

```bash
# Use a BusyBox ELF from firmware you personally own and have permission to inspect.
export HGFW_STOCK_BUSYBOX=/path/to/your/extracted/rootfs/bin/busybox
make compat
```

This *optional* archive incorporates the locally provided stock BusyBox; it is **not** redistributed by this project. It remains nonbootable and must not be flashed.

## Source layout

```text
.
├── .github/workflows/ci.yml    # Build and tests on every push/PR
├── Makefile                    # Reproducible host-only commands
├── cmd/hgfw/                  # Native Linux CLI + daemon
├── internal/                  # Admin, device, service and USB modules
├── config/                    # Explicit services + sample network plan
├── fwkit.py                    # Original userspace overlay builder/verifier
├── compat_rootfs.py            # Optional private compatibility build
├── src/
│   ├── arm/hello.S             # Minimal static ARMv7 syscall test
│   ├── etc/hgfw/               # Feature manifest and default settings
│   └── usr/sbin/               # Read-only hardware and USB diagnostics
├── tests/                      # Reproducibility and archive safety tests
└── tools/build_arm.sh          # ARM cross-build with Clang + LLD
```

Build output, extracted proprietary firmware, calibration data, private device settings and passwords are deliberately excluded from version control.

## Roadmap

1. **Hardwar
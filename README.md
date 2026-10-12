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
| Password-protected local dashboard and live device status | Implemented; host-tested, QEMU ARMv7 no-VFP login/API/CSRF/logout smoke test passed; physical dashboard test pending |
| USB device detection and opt-in read-only mount/unmount CLI | Implemented; hardware access not yet tested with HGFW |
| Routed network topology validator (no writes) | Implemented |
| Reproducible userspace overlay + checksums | Working |
| Offline Python and Go test suites | Working |
| Native execution on the HG6245D | **Stage 2 softfloat ARMv7 binary verified on physical BCM96846**: `version`, `status`, and `usb` each returned exit code 0 from RAM-backed tmpfs (Oct 12, 2026). Initial hardfloat build was incompatible; `GOARM=7,softfloat` fixed it. |
| Ethernet WAN, firewall, DHCP and DNS | Planned |
| Wireless client / repeater modes | Under investigation |
| SMB/NAS file sharing | Planned |
| USB printer server | Planned; requires printer compatibility testing |
| Independent admin UI | Implemented read-only preview; router deployment untested |
| Bootable custom image / safe installer | Blocked by boot-chain and recovery validation |

### Loopback dashboard smoke test

Run `hgfw smoke` with the native host executable or the ARMv7 executable. It starts the real admin HTTP handler on an **OS-assigned loopback-only TCP port**, uses a temporary random password held only in memory, and checks anonymous access denial, login, protected dashboard and JSON API, CSRF enforcement, logout revocation, server shutdown, and process memory usage. It writes no credentials, makes no router configuration changes, and prints a JSON summary without passwords or device MAC addresses.

```sh
./build/hgfw-host smoke
# For an HG6245D, run the verified ARM softfloat binary from tmpfs:
# /mnt/hgfw-run/hgfw smoke
```

The full end-to-end smoke test has passed on the **no-VFP ARM emulator**. Execution of the dashboard smoke test on the physical device is the next validation milestone. Because it binds to the router's 127.0.0.1, it is intentionally not browsable from another computer on the LAN.

### Verified non-flashing hardware test

On a physical HG6245D running stock RP2613 Linux 4.1.45, the independently built HGFW Stage 2 ARMv7 binary was SHA-256 verified, copied to a temporary RAM-backed filesystem, and executed without a firmware change. The `version`, `status`, and `usb` commands all completed with exit code 0. `status` identified `Broadcom BCM96846` and enumerated stock Ethernet, switch and dual-band Wi-Fi interfaces; `usb` enumerated a removable disk and partition. The binary and temporary mount were removed afterward.

**Scope:** These were read-only device-inventory commands. This does **not** verify the admin dashboard, process supervisor, USB filesystem mounting or write support, router services, new-kernel boot, or flash recovery. Device-specific network MAC addresses, passwords and raw console transcripts are intentionally not published.

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

1. **Hardware reference:** capture non-sensitive device-tree and kernel configuration structure, map Ethernet/switch functionality.
2. **Userland proof:** execute the ARMv7 test program in a temporary environment and confirm compatibility.
3. **Networking:** validate current interfaces and planned isolated WAN/LAN topologies; implement tested, rollback-protected DHCP/DNS, firewall and Wi-Fi controls.
4. **USB services:** verify the opt-in read-only mount flow on the device; add audited SMB/NFS file sharing and printer server support.
5. **Management:** extend the working authenticated observation dashboard with explicitly authorized safe control operations, logs, backups and update/rollback management.
6. **Firmware release:** implement verified boot/recovery and produce a flashable image only after safe failure recovery is established.

## Safety and legal notes

- **Nothing in this repository should be flashed or installed over an active firmware image.** An unsafe image can permanently prevent software-only recovery.
- Proprietary Broadcom/FiberHome components, original NAND backups, and device-specific credentials are not published.
- The original Linux 4.1 kernel is obsolete; a compatibility approach must not be treated as safe for internet-facing production deployment.
- HGFW is an independent community project, **not affiliated with or endorsed by FiberHome, Broadcom, or PLDT**.
- No license has been selected yet. Until a license is added, the default copyright rules apply to these source files.

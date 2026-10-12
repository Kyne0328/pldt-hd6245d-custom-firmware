# HG6245D RP2613: offline boot chain and hybrid firmware engineering

**Evidence cut:** 2026-10-12 (Asia/Manila). Device: owner-authorized FiberHome HG6245D RP2613, board `WKE2.094.277A01`, SoC Broadcom BCM96846, stock Linux 4.1.45 ARMv7.

**Critical boundary:** This describes **offline** component reconstruction. No image in this work is a proven bootable HG6245D image, a valid updater container, or a verified NAND replacement. No flash, reboot, bootloader or router-side command was executed as part of this milestone.

## 1. Source material, provenance and preservation

The local owner-supplied original backup consists of data-area-only NAND captures:

| Original partition | Bytes | Observed purpose |
| --- | ---: | --- |
| mtd0 rootfsA | 59,244,544 | UBI image slot A |
| mtd1 rootfsB | 59,244,544 | UBI image slot B |
| mtd2 data | 10,485,760 | UBI data volume, mutable |
| mtd3 nvram | 1,048,576 | Factory/device values; private |
| mtd4 PreConfigure | 2,097,152 | Factory/device values; private |
| mtd5 UserLocalCT | 1,048,576 | Device-local data; private |

Prior evidence recorded UBI headers at 128 KiB eraseblock intervals (2048-byte VID offset and 4096-byte data offset), and volumes `rootfs_ubifs`, `app_ubifs`, `filestruct_full.bin`, `METADATA`, `METADATACOPY`, plus a separate data volume. The backups omit NAND OOB/ECC spare bytes, boot ROM, and verified complete bootloader/recovery material; they are **not** raw-flash recovery images.

The two community-source vendor download packages were kept unchanged:

| Image | Bytes | SHA256 | Listed board revisions |
| --- | ---: | --- | --- |
| RP2725 | 39,121,687 | `5ac7d87efafca23605c2a1aaa1f2875300edb7546f57055e85afeff740e49428` | Includes `WKE2.094.277A01` |
| RP2726 | 39,123,481 | `e491b5f878758d98590eac560334cade528b44c8f0599988a8358b4a4f466747` | Does not list `WKE2.094.277A01` |

The packages are vendor-formatted updates, not raw UBI/NAND images. No vendor publisher signature, verified compatibility beyond explicit identifiers, upgrade authorization policy, or tested recovery guarantee has been established. Do **not** flash either one.

## 2. Reconstructed boot-file container

The original RP2613 `filestruct_full.bin` volume uses a record list. Each observed record has four big-endian 32-bit header words (record length, record type, payload length, still-unverified opaque value), a NUL-terminated filename, a payload, and a further 32-bit opaque trailer. The record ordering is:

| Record | Type | RP2613 slot A bytes | RP2725 bytes | Same payload? |
| --- | ---: | ---: | ---: | --- |
| `96846.dtb` | 18 | 2,348 | 2,348 | Yes, byte-for-byte |
| `cferam.000` | 19 | 763,616 | 763,716 | No |
| `vmlinux.lz` | 19 | 2,216,046 | 2,290,550 | No |
| `image_ident` | 20 | 110 | 110 | No |
| `image_version` | 22 | 33 | 33 | No |

These values were established by local structural parsing, not by assuming magic bytes mark top-level firmware files. The RP2725 and RP2726 packages contain the same candidate `96846.dtb` and `cferam.000`, but distinct `vmlinux.lz` payloads. The board DTB also exactly matches the original RP2613 slot A and slot B firmware backups. **Only** a contiguous 4,364-byte region around the DTB matched at the corresponding positions; the full kernels are not identical.

The `vmlinux.lz` record starts with a Broadcom-specific `BRCM` wrapper; it is not an ordinary standalone `gzip` or `uImage` file. The opaque record fields have not been independently authenticated as checksums or signatures. It is unsafe to synthesize an updater from this record list without reconstructing the full vendor envelope and verification logic.

The original `cferam.000` payload gives useful CFE-related research material but **does not** constitute an independently verified boot ROM/bootstrap dump or a proven Ethernet recovery path.

## 3. Recovered userspace startup dependency chain

From the offline original `rootfs_ubifs` extraction:

1. `/etc/inittab` invokes `/etc/init.d/rcS` as sysinit.
2. `rcS` enumerates executable `/etc/rc3.d/S??*` startup links.
3. The original UBIFS includes `S25mount-fs`, `S35system-config`, `S63save-dmesg` (and a shutdown link). These were **lost** in the older NTFS extraction.
4. `mount-fs.sh` reads a kernel driver boot flag and attempts to mount application volume `ubi0_3` or `ubi1_3` under `/fhrom`, with fallbacks. The device data volume is mounted at `/fhconf` when available.
5. Essential proprietary modules are present under `/lib/modules/4.1.45/extra`, including `wl.ko` (Broadcom Wi-Fi), `rdpa.ko` (Broadcom runner/data-path) and GPON modules. These depend on the original kernel ABI and stock hardware initialization.

This identifies the dependencies but does not verify the complete live boot chain, actual module initialization order, application service readiness, or behavior when any component is replaced.

## 4. POSIX-correct UBIFS extraction: important correction

The old Windows-mounted extraction contained hundreds of files but **zero** symlinks and gave BusyBox and init scripts permissions that were not executable. It cannot support reliable boot-image reconstruction.

Using the preserved UBI logical volume and a Linux-compatible `ubi_reader` with `lzallright`, `zstandard`, and `cryptography`, we recovered an unprivileged Linux filesystem tree with:

- **429 regular files, 310 symbolic links**, and recoverable original POSIX read/write/execute bits.
- `bin/busybox` and `etc/init.d/rcS` mode `0755`; `S25mount-fs` present as its original symlink.
- **803 indexed UBIFS inodes, all UID 0 and GID 0**; zero bad logical eraseblocks reported by the index walk.
- No root execution, `chown`, setuid/setgid restoration, device-node creation, or NAND operations during the new extraction. Original xattrs and security contexts have not been independently verified.

Private WSL source directory: `$HOME/hgfw-rp2613-rootfs-modes`.

## 5. Generated offline deliverables

All outputs listed below are local-only; they are NOT public GitHub releases.

| Local deliverable | Result | Status |
| --- | --- | --- |
| `build/hgfw-armv7-next` | 7,667,874 bytes, ARMv7 softfloat | Tested under ARM user-mode QEMU; existing HGFW components hardware-tested in earlier builds |
| `dist/private-initramfs/hgfw-experimental-initramfs-armv7.cpio.gz` | 3,958,039 bytes, 23 entries, SHA256 `40c303f5f63c2d0511aad6a6123d106f342634a4a2fd2d1a848b15b16538d07f` | Standalone experimental userspace stage, **no kernel/drivers or confirmed boot** |
| `$HOME/hgfw-hybrid-rootfs-posix` | 362 files, 307 symlinks, 56 directories, 102 recovered `.ko` files (including added HGFW) | Private integration staging; no boot or install tested |
| `$HOME/hgfw-hybrid-rootfs-archive/hgfw-rp2613-hybrid-rootfs-private.tar.gz` | 14,838,819 bytes; SHA256 `6cbe890fa78d79bc8f36a065b33fd9bc2c8034ee916e534d16a2e7f7f5e6cdd6` | Deterministic TAR retaining modes, relative symlinks, and observed root:root ownership; **not a UBIFS image** |

The private hybrid stage keeps compatible stock userspace/driver files but **does not** include customer provisioning, the independent app UBI volume, bootloader, kernel, verified xattrs, or a startup takeover. It adds HGFW for manual execution only.

The experimental initramfs includes no network autostart. A shell-only initramfs on a router without a verified console/recovery channel is unsuitable for deployment, even if it can pass host CPIO and ARM executable checks.

## 5a. Independent RP2613 application-volume recovery

A subsequent native-Linux read-only recovery of the original `app_ubifs` logical image produced **1,200 regular files and three symbolic links**. Its UBIFS index recorded **1,677 inodes, every one with UID 0 and GID 0**, and zero bad logical eraseblocks in the index walk. The application tree includes `/bin`, `/lib`, `/fhshell`, and an independent `/fhconf` subtree; the latter was **excluded** from the application compatibility staging. The original app source was not modified.

An independent private application staging tree at `$HOME/hgfw-app-compat-posix` contains **1,084 files and three symlinks** (1,535 total entries including directories). Its reproducible TAR is `$HOME/hgfw-app-compat-archive/hgfw-rp2613-app-compat-private.tar.gz`, **11,472,278 bytes**, SHA-256 `310418a0399644cb8cc7783080a5e303fad2c7cf1ae0066629957711649bbc54`. It preserves mode bits and symlinks, with owner `root:root` established by the UBIFS index. Like the hybrid rootfs TAR, it is **not** an installable UBI volume and is private, containing third-party stock binaries.

The recovered app volume improves completeness of our offline dependency analysis but does **not** imply stock application startup can be replicated safely: original active-slot selection, `/fhrom` mount, cross-volume libraries, dynamically provisioned `/fhconf`, xattrs/security contexts, and process orchestration still require work.

The fail-closed `tools/assess_boot_readiness.py` validates hashes and manifests for the three current offline artifacts (initramfs, hybrid rootfs TAR, application-compatibility TAR) and explicitly reports **six independent hardware/release blockers**. Local artifact checks passing never lifts the installation block.

## 6. What blocks a full installable firmware

**Device installation remains blocked by independent hardware evidence:**

- **No verified recoverable boot path:** the only known working router has no UART recovery, no demonstrated RAM-only CFE/uImage/initramfs launch path and no tested RP2613 Ethernet recovery procedure. A badly flashed NAND image could permanently strand it.
- **Vendor update semantics unresolved:** the complete update envelope, signature/CRC policy, boot-slot selection and required rollback flags have not been safely validated on RP2613.
- **Partition semantics:** original UBIFS application and data volumes and the private factory/provisioning structures must be preserved; two 57 MiB UBI slots do not map directly to a 39 MiB downloadable updater.
- **Drivers and service parity:** the kernel ABI, Broadcom Wi-Fi/GPON/runner modules, proprietary application startup, LAN switching and WAN provisioning have no independent end-to-end replacement test. QEMU user-mode does not emulate this board or its drivers.
- **Full rootfs/app metadata:** both original UBIFS inode sets have been observed as root:root and their permission bits and symlinks recovered. Remaining gaps include xattrs/security contexts, bootable UBI packaging and safe integration of the separately staged application volume.

These are external hardware and vendor-recovery barriers to marking a package bootable/flashable, **not** grounds to stop non-destructive userspace development. A spare board with observed CFE/RAM boot behavior, or an authenticated matching Ethernet recovery method, would unlock the next hardware phase.

## 7. Build and validation entrypoints

```sh
# Source-only tools; do not run anything on an MTD device.
GO=/path/to/go QEMU_ARM=/path/to/qemu-arm-static sh tools/build_release.sh
python3 tools/build_initramfs.py build
python3 tools/build_initramfs.py verify

# Requires native POSIX UBIFS extraction and restored permission-bit attestation.
python3 tools/stage_hybrid_rootfs.py verify --output $HOME/hgfw-hybrid-rootfs-posix
python3 tools/package_hybrid_rootfs.py verify --output $HOME/hgfw-hybrid-rootfs-archive --profile rootfs
python3 tools/stage_stock_app.py verify --output $HOME/hgfw-app-compat-posix
python3 tools/package_hybrid_rootfs.py verify --output $HOME/hgfw-app-compat-archive --profile app
python3 tools/assess_boot_readiness.py --initramfs dist/private-initramfs --rootfs $HOME/hgfw-hybrid-rootfs-archive --app $HOME/hgfw-app-compat-archive --output dist/private-initramfs/boot-readiness.json
```

No file labeled `*.cpio.gz` or `*.tar.gz` in these private directories may be sent to the HG6245D updater or directly written to flash. The source repository intentionally excludes all original proprietary binary payloads and user-specific firmware dumps.

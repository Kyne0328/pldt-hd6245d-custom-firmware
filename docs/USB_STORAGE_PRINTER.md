# USB storage and printer integration

**Target:** First-party HGFW for FiberHome HG6245D RP2613. **Status:** implemented and locally tested; device-specific USB hardware tests still pending. All external services are disabled by default. Existing PLDT GPON/WAN/Wi-Fi configuration is never changed by these commands.

## USB storage

HGFW reads kernel-confirmed USB block devices from `/sys/class/block` and `/proc/mounts`, distinguishes disks from partitions and shows mounted filesystems in the authenticated **USB** page. The user must deliberately mount a verified partition:

```sh
# Mount root must exist and not be a symlink; an example only, do not run without the USB device confirmed:
hgfw usb-mount --partition /dev/sda1 --mount-root /mnt/hgfw --confirm
```

It mounts with `ro,nosuid,nodev,noexec` flags and never formats or writes the filesystem. Once a USB partition is mounted under `/mnt/hgfw/sda1`, an authenticated dashboard user can **Browse** folders or download regular files (maximum 32 MiB each) through `/api/storage/list` and `/api/storage/file`. The backend validates the partition against actual kernel USB-block and mount metadata, confines path operations using `os.OpenRoot`, rejects traversal/out-of-root symlinks and hides dotfiles and special files. No public file server, SMB daemon, FTP account or write API is started. Downloads require an active authenticated management session.

Unmount only after closing active file downloads:

```sh
hgfw usb-unmount --path /mnt/hgfw/sda1 --mount-root /mnt/hgfw --confirm
```

Filesystem compatibility (including NTFS, exFAT and ext4), hot-unplug behavior and throughput depend on the physical stock kernel and available modules; they have not all been verified on HG6245D hardware.

## USB printer

The stock RP2613 root filesystem contains `usblp.ko`. HGFW detects a real USB printer only when the Linux USB printer class and a character device `/dev/usb/lpN` or `/dev/lpN` are both present. `hgfw printer-list` is read-only.

The independent print bridge accepts *already formatted* raw printer-language jobs over TCP (e.g. PCL or PostScript, where supported by that printer). It is not CUPS, IPP or a universal printer driver, and cannot translate arbitrary documents to printer-compatible output.

A deliberately enabled, limited LAN session can be tested only **after** an actual printer is identified and an authorized operator approves the address, subnet and port:

```sh
hgfw printer-list
# Example LAN configuration; change values to actual assigned IPv4 and verified printer device:
hgfw printer-serve --listen 192.168.1.2:9100 --allow-subnet 192.168.1.0/24 --device /dev/usb/lp0 --max-kib 512 --jobs 1 --seconds 300 --confirm
```

The service refuses wild-card/public listeners, rejects clients outside the selected subnet, limits each job to 4 MiB or less, buffers a complete job before writing to the printer, stops after the configured job count or duration, and does not start at boot. **Raw port 9100 is unauthenticated and unencrypted**; expose it only briefly to a trusted isolated LAN, never to the WAN. USB printer class detection, actual printed pages, driver initialization and LAN client compatibility still require a connected printer test on the physical device.

## Dashboard wording

The original local FiberHome ASP files informed the menu labels: **Device Information**, **Wireless State**, **WAN State**, **LAN State**, **Optical Power**, **DHCP Client List**, **Samba** and **USB**. HGFW reimplements its own authenticated UI; it does not reuse or invoke vendor ASP/`goform` write endpoints. Debug provenance lines, build-stage tags and repeated engineering copy were removed from routine views. Unsupported configuration choices still show **Unavailable** rather than pretending to work.

## Validation and limitations

- Go unit tests cover USB mount-tree validation, traversal and symlink rejection, printer option validation, bounded jobs, temporary loopback TCP printing, and unauthenticated download denial.
- ARMv7 softfloat builds and QEMU selftests verify the program structure, not Broadcom's physical USB bus, printers or installed filesystem drivers.
- SMB/Samba serving, read-write file sharing, printer discovery to PCs, IPP, spooling, and a fully independent GPON/Wi-Fi/router boot path remain unimplemented or unverified.
- None of these USB features makes the experimental initramfs or hybrid TAR installable firmware. Boot, update-envelope compatibility and recovery remain blocked pending a proven RP2613 path.

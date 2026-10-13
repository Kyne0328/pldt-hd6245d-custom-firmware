# RP2613 experimental UBI firmware slot

**Board:** FiberHome HG6245D, `WKE2.094.277A01`, stock RP2613, Broadcom BCM96846 ARMv7 soft-float.  
**Build:** October 13, 2026. **Classification:** verified *offline container*, **NOT verified bootable or flashable**.

## Constructed artifact (PRIVATE)

`$HOME/hgfw-offline-tools/ubi/private-slot-20261013/hgfw-rp2613-experimental-slot.ubi`

- 50,200,576 bytes (383 physical eraseblocks).
- SHA-256: `9739f92675df5b2d15871c4198185f5c0bd45e11b9729e5b9363a5628c07987d`.
- UBI geometry: PEB 131,072 bytes, minimum I/O 2,048 bytes, VID offset 2,048, LEB 126,976 bytes; original physical slot spans 452 PEBs.
- Original UBI image sequence: `1080062015`.
- Contains proprietary stock firmware, possibly private device configuration; **never publish this artifact**.

| UBI ID | Volume name | Type | Reserved LEB | Payload |
| --- | --- | --- | ---: | --- |
| 0 | `rootfs_ubifs` | dynamic | 160 | Original RP2613 rootfs, byte-identical |
| 1 | `METADATA` | dynamic | 1 | Original RP2613 volume metadata, byte-identical |
| 2 | `METADATACOPY` | dynamic | 1 | Original RP2613 metadata copy, byte-identical |
| 3 | `app_ubifs` | dynamic | 240 | Newly constructed original app + HGFW executable |
| 10 | `filestruct_full.bin` | static | 31 | Original RP2613 boot-file records, byte-identical |

The updated application volume occupies 195 of 240 reserved LEBs. It retains the complete original locally recovered application files, including device-specific configuration material in this **private** experimental staging; HGFW is added at `/bin/hgfw` within the application filesystem (normally `/fhrom/bin/hgfw` after the stock mount). There is **no HGFW autostart hook**, because its credential provisioning, TLS and stock-service concurrency requirements have not been established.

The stock rootfs is unchanged in this candidate. An alternative rebuilt rootfs with HGFW in `/usr/bin` failed the original 160-LEB limit under LZO compression (minimum 175 LEB). A zlib-compressed rootfs variant did fit offline, but that variant is **not** in the validated UBI candidate and has not been verified against the physical stock kernel. Leaving the stock boot rootfs intact is the less invasive option.

## Reproducibility and integrity

- `tools/build_app_ubifs.py` creates a private app volume with `mkfs.ubifs -m 2048 -e 126976 -c 240 -x lzo -U`. It does not touch NAND and requires a separately attested owner-metadata flag.
- `tools/build_private_slot_ubi.py` uses `ubinize` with exact five UBI volume names/IDs/reservations and preserved stock payloads. Outputs are refused inside the public source tree or if the destination exists.
- `ubi_reader` parsed the generated image and reported all five expected volume IDs, names and reserved block counts.
- An independent `ubi_reader` extraction reproduced all **five volume payloads byte-for-byte**, including the newly built `app_ubifs` and all four preserved stock volumes.
- Python unit tests cover volume geometry, required vendor metadata, bounds checking, non-UBIFS inputs, out-of-repository protection and explicit private-data ownership attestation.

The UBI container hash represents this specific build. Filesystem timestamps or toolchain versions may cause rebuilt images to differ byte-for-byte, even when volume contents are functionally equivalent.

## Blocking requirements before a genuine HGFW firmware release

1. **Bootloader acceptance and vendor update envelope.** The stock CFE boot selection, signing/checksum policy and upgrade packaging are not verified. A UBI image is not a vendor `.bin` firmware updater payload.
2. **Verified rescue procedure.** No proven RP2613 Ethernet/UART recovery from a nonbooting firmware image; currently available NAND dumps exclude OOB/ECC and complete bootloader.
3. **HGFW startup.** A protected credential/TLS initialization and service takeover sequence must be implemented and tested with the actual stock kernel and vendor service dependencies.
4. **Full hardware behavior.** Broadband GPON/OMCI, Ethernet switch/VLAN, Wi-Fi radio/bridging, routing/firewall, USB printer and USB storage operation require actual board tests. User-mode ARM QEMU does not emulate BCM96846 peripherals.
5. **Provisioning preservation.** Customer optical identity, calibration and private NVRAM/data partition behavior require a tested no-loss migration and rollback plan.
6. **Power-loss and dual-slot behavior.** Updated slot selection, UBI bad-block handling and interrupted-update recovery must be tested, preferably on a spare device.

**DO NOT FLASH OR SEND THIS UBI IMAGE TO THE ROUTER.** All evidence so far is offline component/container testing. The user's working router was not rebooted or modified during this build.

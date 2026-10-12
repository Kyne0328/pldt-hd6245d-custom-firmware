#!/bin/sh
# HGFW complete OFFLINE engineering candidate. Runs only on the developer PC.
# Never connects to a router and never produces an MTD/UBI firmware update.
set -eu

cd "$(dirname "$0")/.."
OUT="${HGFW_PRIVATE_OUT:-dist/private-initramfs}"
BUSYBOX="${HGFW_STOCK_BUSYBOX:-../analysis/extracted_mtd0/1080062015/rootfs_ubifs/bin/busybox}"
PYTHON="${PYTHON:-python3}"
GO="${GO:-go}"
export GO

echo "[HGFW] First-party tests, ARM build and QEMU tests"
sh tools/build_release.sh

echo "[HGFW] Reproducible offline RAM boot-stage package"
"$PYTHON" tools/build_initramfs.py build --busybox "$BUSYBOX" --hgfw build/hgfw-armv7-next --output-dir "$OUT"
"$PYTHON" tools/build_initramfs.py verify --output-dir "$OUT"

if [ -n "${HGFW_VENDOR_IMAGE:-}" ]; then
  echo "[HGFW] Inspect vendor image metadata, never write firmware"
  if [ -n "${HGFW_ORIGINAL_FILESTRUCT:-}" ]; then
    "$PYTHON" tools/inspect_vendor_image.py "$HGFW_VENDOR_IMAGE" --reference "$HGFW_ORIGINAL_FILESTRUCT" --output "$OUT/vendor-image-comparison.json"
  else
    "$PYTHON" tools/inspect_vendor_image.py "$HGFW_VENDOR_IMAGE" --output "$OUT/vendor-image-comparison.json"
  fi
fi

echo "[HGFW] Verify CPIO reader independently"
gzip -cd "$OUT/hgfw-experimental-initramfs-armv7.cpio.gz" | cpio -it >/dev/null

echo "[HGFW] Verify ARM binaries can execute without the vendor rootfs"
if [ -n "${QEMU_ARM:-}" ] && [ -x "${QEMU_ARM}" ]; then
  "$QEMU_ARM" "$BUSYBOX" true
  "$QEMU_ARM" build/hgfw-armv7-next version
else
  echo "QEMU_ARM_UNSET: ARM execution checks not performed"
fi

echo "[HGFW] Private stock-rootfs integration (no boot or startup takeover)"
HYBRID_OUT="${HGFW_HYBRID_OUT:-dist/private-hybrid-rootfs}"
if [ -d "$HYBRID_OUT" ]; then
  "$PYTHON" tools/stage_hybrid_rootfs.py verify --output "$HYBRID_OUT"
  "$PYTHON" - "$HYBRID_OUT" <<'PY'
import hashlib, pathlib, sys
root=pathlib.Path(sys.argv[1])
a=(root/'rootfs/usr/bin/hgfw').read_bytes()
b=pathlib.Path('build/hgfw-armv7-next').read_bytes()
if hashlib.sha256(a).digest()!=hashlib.sha256(b).digest():
    raise SystemExit('EXISTING_STAGE_STALE: use a new HGFW_HYBRID_OUT (never overwrite original staging)')
PY
else
  if [ "${HGFW_STOCK_MODES_RESTORED:-0}" = "1" ]; then
    "$PYTHON" tools/stage_hybrid_rootfs.py stage --rootfs "${HGFW_STOCK_ROOTFS:-../analysis/extracted_mtd0/1080062015/rootfs_ubifs}" --output "$HYBRID_OUT" --mode-bits-from-ubifs
  else
    "$PYTHON" tools/stage_hybrid_rootfs.py stage --rootfs "${HGFW_STOCK_ROOTFS:-../analysis/extracted_mtd0/1080062015/rootfs_ubifs}" --output "$HYBRID_OUT"
  fi
  "$PYTHON" tools/stage_hybrid_rootfs.py verify --output "$HYBRID_OUT"
fi

MODE_BITS=$($PYTHON - "$HYBRID_OUT" <<'PY'
import json,pathlib,sys
m=json.loads((pathlib.Path(sys.argv[1])/'manifest.json').read_text())
print('yes' if m.get('modeBitsRestoredFromUBIFS') else 'no')
PY
)
if [ "$MODE_BITS" = yes ] && [ "${HGFW_ORIGINAL_UBIFS_OWNERS_VERIFIED:-0}" = "1" ]; then
  echo "[HGFW] Deterministic private rootfs archive (not flashable)"
  HYBRID_ARCHIVE="${HGFW_HYBRID_PACKAGE_OUT:-dist/private-hybrid-rootfs-archive}"
  "$PYTHON" tools/package_hybrid_rootfs.py build --rootfs "$HYBRID_OUT/rootfs" --output "$HYBRID_ARCHIVE" --all-root-owners-verified
  "$PYTHON" tools/package_hybrid_rootfs.py verify --output "$HYBRID_ARCHIVE"
else
  echo "PRIVATE_ROOTFS_ARCHIVE_SKIPPED: original UBIFS modes and root ownership not both attested"
fi

if [ -n "${HGFW_STOCK_APP:-}" ] && [ "${HGFW_ORIGINAL_APP_OWNERS_VERIFIED:-0}" = "1" ]; then
  echo "[HGFW] Separate RP2613 application UBI volume, private and with provisioning excluded"
  APP_STAGE="${HGFW_APP_OUT:-dist/private-app-compat}"
  APP_PACKAGE="${HGFW_APP_PACKAGE_OUT:-dist/private-app-archive}"
  if [ -d "$APP_STAGE" ]; then
    "$PYTHON" tools/stage_stock_app.py verify --output "$APP_STAGE"
  else
    "$PYTHON" tools/stage_stock_app.py stage --source "$HGFW_STOCK_APP" --output "$APP_STAGE" --all-root-owners-verified
  fi
  "$PYTHON" tools/package_hybrid_rootfs.py build --rootfs "$APP_STAGE/rootfs" --output "$APP_PACKAGE" --profile app --all-root-owners-verified
  "$PYTHON" tools/package_hybrid_rootfs.py verify --output "$APP_PACKAGE" --profile app
fi

echo "[HGFW] Complete offline output manifest"
"$PYTHON" - "$OUT" <<'PY'
import hashlib, json, pathlib, sys
out = pathlib.Path(sys.argv[1])
manifest = json.loads((out / "hgfw-experimental-initramfs-manifest.json").read_text())
files = {}
for path in (pathlib.Path("build/hgfw-armv7-next"), out / "hgfw-experimental-initramfs-armv7.cpio.gz"):
    blob = path.read_bytes()
    files[path.name] = {"bytes": len(blob), "sha256": hashlib.sha256(blob).hexdigest()}
report = {
    "project": "HGFW",
    "stage": "offline-boot-stage-only",
    "platform": "brcm,bcm96846",
    "readyForDeviceFlash": False,
    "readyForProductionBoot": False,
    "requiresVerifiedBootloaderRecovery": True,
    "containsIndependentKernel": False,
    "containsBoardDrivers": False,
    "containsLiveNetworkServices": False,
    "containsPrivateVendorBinary": True,
    "doNotPublishArchive": True,
    "files": files,
    "entrypoint": manifest["entrypoint"],
}
(out / "candidate-readiness.json").write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
print("CANDIDATE_READY_OFFLINE=true")
print("FLASH_READY=false")
for name, info in files.items():
    print(f"{name}: {info['bytes']} bytes SHA256={info['sha256']}")
PY

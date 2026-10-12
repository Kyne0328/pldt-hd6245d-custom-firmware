#!/usr/bin/env python3
"""Offline-only assembly of a PRIVATE RP2613 hybrid rootfs staging tree.

Copies the owner's already-extracted baseline into a *new* output folder and
adds HGFW as an optional manual executable. Does not patch the original
firmware, install hooks, alter stock init, build NAND images or access a router.

The source extraction on NTFS does not preserve original UNIX ownership and
modes; this staging tree cannot be trusted as a bootable reconstruction.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_ORIGINAL = ROOT.parent / "analysis/extracted_mtd0/1080062015/rootfs_ubifs"
DEFAULT_BIN = ROOT / "build/hgfw-armv7-next"
DEFAULT_OUT = ROOT / "dist/private-hybrid-rootfs"

# Factory, provisioned, personally identifying and mutable mount content must
# not be included in this development staging tree.
DENIED = (
    "fhconf", "fhdata", "fhct", "preconf", "var/log", "var/run", "var/state",
    "etc/shadow", "etc/passwd", "etc/group", "etc/ppp", "etc/wlan",
    "etc/ssh", "etc/ssl", "etc/telephonyProfiles.d", "root", "home",
)
BOOT_NOTE = (
    "HGFW MANUAL HYBRID STAGING ONLY\n"
    "The original RP2613 stock init scripts and kernel modules are retained as data.\n"
    "No automatic HGFW startup, stock services takeover or NAND update is configured.\n"
    "Private provisioning paths are intentionally excluded, so this tree\n"
    "cannot be booted to replace the original firmware.\n"
    "Original file permissions and owners were not recovered from the NTFS extraction.\n"
    "Never flash, chroot, mount over root, or run stock init from this staging tree.\n"
)


def denied(relative):
    s = relative.as_posix()
    return any(s == p or s.startswith(p + "/") for p in DENIED)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def inventory(tree):
    result = []
    for path in sorted(tree.rglob("*")):
        rel = path.relative_to(tree)
        if denied(rel):
            raise ValueError(f"private path unexpectedly included: {rel}")
        if path.is_symlink():
            row = {"path": rel.as_posix(), "kind": "symlink", "target": os.readlink(path)}
        elif path.is_dir():
            row = {"path": rel.as_posix(), "kind": "directory"}
        elif path.is_file():
            body = path.read_bytes()
            row = {"path": rel.as_posix(), "kind": "file", "bytes": len(body), "sha256": sha(body)}
        else:
            raise ValueError(f"unexpected special node: {rel}")
        row["mode"] = oct(path.lstat().st_mode & 0o777)
        result.append(row)
    return result


def stage(original, executable, output, mode_bits_from_ubifs=False):
    original = Path(original).resolve(strict=True)
    executable = Path(executable).resolve(strict=True)
    output = Path(output).absolute()
    if output.exists() or output.is_symlink():
        raise FileExistsError("refusing to overwrite existing staging tree")
    if original == output or original in output.parents or output in original.parents:
        raise ValueError("unsafe recursive or overlapping directory")
    if not original.is_dir() or not (original / "bin/busybox").is_file():
        raise ValueError("not a verified stock rootfs extraction")
    binary = executable.read_bytes()
    if binary[:7] != b"\x7fELF\x01\x01\x01" or len(binary) < 500000:
        raise ValueError("not a built ARM HGFW executable")

    def ignore(directory, names):
        base = Path(directory)
        omitted = set()
        for name in names:
            path = base / name
            rel = path.relative_to(original)
            if denied(rel):
                omitted.add(name)
        return omitted

    rootfs = output / "rootfs"
    rootfs.parent.mkdir(parents=True, exist_ok=True)
    shutil.copytree(original, rootfs, symlinks=True, ignore=ignore)
    hgfw_file = rootfs / "usr/bin/hgfw"
    hgfw_file.parent.mkdir(parents=True, exist_ok=True)
    hgfw_file.write_bytes(binary)
    os.chmod(hgfw_file, 0o755)
    notes = rootfs / "etc/hgfw"
    notes.mkdir(parents=True, exist_ok=True)
    (notes / "README").write_text(BOOT_NOTE, encoding="utf-8")
    services = json.loads((ROOT / "config/services.json").read_text())
    if services != {"services": []}:
        raise ValueError("services are not disabled by default")
    (notes / "services.json").write_text(json.dumps(services, sort_keys=True) + "\n", encoding="utf-8")

    members = inventory(rootfs)
    manifest = {
        "schema": 1,
        "kind": "private-stock-rootfs-with-manual-hgfw",
        "bootable": False,
        "flashable": False,
        "automaticallyStartsHGFW": False,
        "vendorUserspaceContainsProprietarySoftware": True,
        "ownershipAndModeMetadataVerified": False,
        "modeBitsRestoredFromUBIFS": bool(mode_bits_from_ubifs),
        "kernelIncluded": False,
        "separateApplicationUBIVolumeIncluded": False,
        "factoryProvisioningIncluded": False,
        "unsafeToRunStockInitInStaging": True,
        "files": members,
    }
    (output / "manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print("STAGED_FILES", sum(x["kind"] == "file" for x in members))
    print("STAGED_SYMLINKS", sum(x["kind"] == "symlink" for x in members))
    print("STAGING_OUTPUT", output)
    print("FLASH_READY=false")
    return manifest


def verify(output):
    out = Path(output).absolute()
    manifest = json.loads((out / "manifest.json").read_text())
    if manifest["flashable"] or manifest["bootable"] or manifest["automaticallyStartsHGFW"]:
        raise ValueError("unapproved staging state")
    actual = inventory(out / "rootfs")
    if actual != manifest["files"]:
        raise ValueError("staging inventory differs from manifest")
    print("VERIFIED_HYBRID_STAGING", len(actual))
    return True


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("action", choices=("stage", "verify"))
    p.add_argument("--rootfs", type=Path, default=DEFAULT_ORIGINAL)
    p.add_argument("--hgfw", type=Path, default=DEFAULT_BIN)
    p.add_argument("--output", type=Path, default=DEFAULT_OUT)
    p.add_argument("--mode-bits-from-ubifs", action="store_true")
    opt = p.parse_args()
    if opt.action == "stage":
        stage(opt.rootfs, opt.hgfw, opt.output, opt.mode_bits_from_ubifs)
    else:
        verify(opt.output)


if __name__ == "__main__":
    main()

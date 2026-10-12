#!/usr/bin/env python3
"""Package a reconstructed PRIVATE RP2613 hybrid rootfs as deterministic TAR.

This is a host-only payload for compatibility research. It is NOT a NAND image,
not a UBIFS volume, not a bootloader-compatible installer and must not be
distributed because it contains proprietary stock binaries. All stock rootfs
inode owners were independently observed to be root:root in the RP2613 sample,
but restoring ownership to a tar header does not validate boot readiness.
"""
import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import stat
import tarfile

NAME = "hgfw-rp2613-hybrid-rootfs-private.tar.gz"
MANIFEST = "hgfw-rp2613-hybrid-rootfs-private.json"
APP_NAME = "hgfw-rp2613-app-compat-private.tar.gz"
APP_MANIFEST = "hgfw-rp2613-app-compat-private.json"


def names(profile):
    if profile == "rootfs":
        return NAME, MANIFEST
    if profile == "app":
        return APP_NAME, APP_MANIFEST
    raise ValueError("unsupported private UBIFS volume profile")


def sha(data):
    return hashlib.sha256(data).hexdigest()


def members_from_tree(root):
    root = Path(root).resolve(strict=True)
    expected = root.parent / "manifest.json"
    if not expected.is_file():
        raise FileNotFoundError("no verified staging manifest")
    spec = json.loads(expected.read_text(encoding="utf-8"))
    if spec.get("bootable") or spec.get("flashable") or spec.get("factoryProvisioningIncluded"):
        raise ValueError("unsafe or inconsistent staging manifest")
    if not spec.get("modeBitsRestoredFromUBIFS"):
        raise ValueError("source modes were not restored from original UBIFS metadata")
    # Reconstruct a fixed, ordered inventory and verify content before copying.
    import sys
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    from stage_hybrid_rootfs import inventory
    observed = inventory(root)
    if observed != spec["files"]:
        raise ValueError("staged rootfs has changed since attestation")
    rows = []
    for row in observed:
        path = root / row["path"]
        kind = row["kind"]
        mode = int(row["mode"], 8)
        if kind == "symlink":
            data = b""
        elif kind == "file":
            data = path.read_bytes()
        elif kind == "directory":
            data = b""
        else:
            raise ValueError("unsafe tree member type")
        rows.append((row, mode, data))
    return rows


def assemble(root):
    rows = members_from_tree(root)
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w", format=tarfile.PAX_FORMAT) as arc:
        for row, mode, contents in rows:
            name = row["path"]
            if name.startswith("/") or ".." in name.split("/"):
                raise ValueError("unsafe archive path")
            ti = tarfile.TarInfo(name)
            ti.uid = ti.gid = 0
            ti.uname = ti.gname = "root"
            ti.mtime = 0
            ti.mode = mode & 0o777
            if row["kind"] == "file":
                ti.type = tarfile.REGTYPE
                ti.size = len(contents)
                arc.addfile(ti, io.BytesIO(contents))
            elif row["kind"] == "directory":
                ti.type = tarfile.DIRTYPE
                arc.addfile(ti)
            elif row["kind"] == "symlink":
                ti.type = tarfile.SYMTYPE
                ti.linkname = row["target"]
                arc.addfile(ti)
    compressed = io.BytesIO()
    with gzip.GzipFile(fileobj=compressed, filename="", mode="wb", compresslevel=9, mtime=0) as zip_file:
        zip_file.write(buf.getvalue())
    return compressed.getvalue(), [row for row, _, _ in rows]


def build(root, output, all_root_owners_verified=False, profile="rootfs"):
    archive_name, manifest_name = names(profile)
    if not all_root_owners_verified:
        raise ValueError("root:root UBIFS inode ownership must be independently verified before TAR metadata restoration")
    out = Path(output)
    out.mkdir(parents=True, exist_ok=True)
    payload, rows = assemble(root)
    manifest = {
        "schema": 1,
        "stage": "offline-proprietary-hybrid-ubifs-archive",
        "volumeProfile": profile,
        "bootable": False,
        "flashable": False,
        "containsKernel": False,
        "includesApplicationUBIVolume": False,
        "includesFactoryProvisioning": False,
        "originalUBIFSOwnersAllRootVerifiedForMtd0": True,
        "originalPosixModeBitsRecovered": True,
        "manualHGFWOnly": True,
        "privateDoNotPublish": True,
        "archive": {"file": archive_name, "bytes": len(payload), "sha256": sha(payload)},
        "members": rows,
    }
    (out / archive_name).write_bytes(payload)
    (out / manifest_name).write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
    print(f"BUILT_PRIVATE_ROOTFS {len(payload)} bytes / {len(rows)} paths SHA256={sha(payload)}")
    return manifest


def verify(output, profile="rootfs"):
    archive_name, manifest_name = names(profile)
    output = Path(output)
    manifest = json.loads((output / manifest_name).read_text())
    if manifest.get("volumeProfile", "rootfs") != profile:
        raise ValueError("volume profile mismatch")
    payload = (output / archive_name).read_bytes()
    if manifest["bootable"] or manifest["flashable"] or not manifest["privateDoNotPublish"]:
        raise ValueError("unsafe archive claims")
    if manifest["archive"] != {"file": archive_name, "bytes": len(payload), "sha256": sha(payload)}:
        raise ValueError("archive identity mismatch")
    found = []
    with tarfile.open(fileobj=io.BytesIO(payload), mode="r:gz") as arc:
        for entry in arc:
            if (entry.uid != 0 or entry.gid != 0 or entry.mtime != 0 or
                entry.name.startswith("/") or ".." in entry.name.split("/")):
                raise ValueError("unsafe archive member")
            if entry.isfile():
                body = arc.extractfile(entry).read()
                row = {"path": entry.name, "kind": "file", "mode": oct(entry.mode), "bytes": len(body), "sha256": sha(body)}
            elif entry.issym():
                row = {"path": entry.name, "kind": "symlink", "mode": oct(entry.mode), "target": entry.linkname}
            elif entry.isdir():
                row = {"path": entry.name, "kind": "directory", "mode": oct(entry.mode)}
            else:
                raise ValueError("unexpected special member")
            found.append(row)
    if found != manifest["members"]:
        raise ValueError("archive file contents differ from manifest")
    print(f"VERIFIED_PRIVATE_ROOTFS {len(found)} paths SHA256={sha(payload)}")
    return True


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("action", choices=("build", "verify"))
    p.add_argument("--rootfs", type=Path, required=False)
    p.add_argument("--output", type=Path, required=True)
    p.add_argument("--all-root-owners-verified", action="store_true")
    p.add_argument("--profile", choices=("rootfs", "app"), default="rootfs")
    args = p.parse_args()
    if args.action == "build":
        if args.rootfs is None:
            p.error("build requires --rootfs")
        build(args.rootfs, args.output, args.all_root_owners_verified, args.profile)
    else:
        verify(args.output, args.profile)


if __name__ == "__main__":
    main()

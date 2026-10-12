#!/usr/bin/env python3
"""HGFW stage-1 host-only, reproducible package builder. Python >= 3.9.

Never reads configuration secrets or writes to any MTD device. Output is an
overlay tarball, not a bootable or flashable router image.
"""
import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path
import struct
import tarfile

ROOT = Path(__file__).resolve().parent
CANARY = ROOT / "build" / "hgfw-canary"
ARTIFACT = "hgfw-stage2-armv7-overlay.tar.gz"
MANIFEST = "manifest.json"

# Explicit allowlist. Packaging never traverses the original filesystem or backups.
OVERLAY_FILES = {
    "etc/hgfw/features.json": "src/etc/hgfw/features.json",
    "etc/hgfw/hgfw.conf": "src/etc/hgfw/hgfw.conf",
    "usr/sbin/hgfw-status": "src/usr/sbin/hgfw-status",
    "usr/sbin/hgfw-usb": "src/usr/sbin/hgfw-usb",
    "usr/bin/hgfw-canary": "build/hgfw-canary",
    "usr/bin/hgfw": "build/hgfw-armv7",
    "etc/hgfw/services.json": "config/services.json",
    "etc/hgfw/network.example.json": "config/network.example.json",
}
EXECUTABLES = {"usr/sbin/hgfw-status", "usr/sbin/hgfw-usb", "usr/bin/hgfw-canary", "usr/bin/hgfw"}
CHUNK = 1024 * 1024


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def check_canary(data):
    if len(data) < 52 or data[:6] != b"\x7fELF\x01\x01":
        raise ValueError("ARM canary missing ELF32 little-endian header")
    machine = struct.unpack_from("<H", data, 18)[0]
    if machine != 40:
        raise ValueError("ARM canary ELF e_machine is not EM_ARM (40)")


def members():
    result = []
    for name, relpath in sorted(OVERLAY_FILES.items()):
        if name.startswith("/") or ".." in Path(name).parts:
            raise ValueError("unsafe package path")
        file_path = ROOT / relpath
        if not file_path.is_file() or file_path.is_symlink():
            raise FileNotFoundError(
                "missing regular source file: {}. For ARM canary: run tools/build_arm.sh on WSL".format(file_path)
            )
        data = file_path.read_bytes()
        if len(data) > 24 * CHUNK:
            raise ValueError("packaged file unexpectedly large: " + name)
        if name in ("usr/bin/hgfw-canary", "usr/bin/hgfw"):
            check_canary(data)
        mode = 0o755 if name in EXECUTABLES else 0o644
        result.append((name, data, mode))
    return result


def reproducible_archive(entries):
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w", format=tarfile.USTAR_FORMAT) as tar:
        for name, data, mode in entries:
            info = tarfile.TarInfo(name=name)
            info.uid = info.gid = 0
            info.uname = info.gname = "root"
            info.mode = mode
            info.size = len(data)
            info.mtime = 0
            info.type = tarfile.REGTYPE
            tar.addfile(info, io.BytesIO(data))
    compressed = io.BytesIO()
    with gzip.GzipFile(filename="", fileobj=compressed, mode="wb", mtime=0, compresslevel=9) as stream:
        stream.write(buffer.getvalue())
    return compressed.getvalue()


def expected_manifest(entries, archive_data):
    return {
        "schema": 1,
        "project": "HGFW",
        "stage": "2-managed-userspace-preview",
        "target": {"platform": "brcm,bcm96846", "cpu": "ARMv7", "kernel": "stock 4.1.45 (reference)"},
        "bootable": False,
        "flashable": False,
        "autoload": False,
        "stock_files_copied": False,
        "archive": {"file": ARTIFACT, "sha256": sha256(archive_data), "bytes": len(archive_data)},
        "files": [
            {"path": name, "bytes": len(data), "sha256": sha256(data), "mode": oct(mode)}
            for name, data, mode in entries
        ],
    }


def build(output_dir=None):
    out = Path(output_dir) if output_dir else ROOT / "dist"
    entries = members()
    data = reproducible_archive(entries)
    manifest = expected_manifest(entries, data)
    out.mkdir(parents=True, exist_ok=True)
    (out / ARTIFACT).write_bytes(data)
    (out / MANIFEST).write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    (out / (ARTIFACT + ".sha256")).write_text(
        "{}  {}\n".format(sha256(data), ARTIFACT), encoding="ascii"
    )
    print("BUILT", out / ARTIFACT, "({} bytes)".format(len(data)))
    return manifest


def verify(output_dir=None):
    out = Path(output_dir) if output_dir else ROOT / "dist"
    archive_file = out / ARTIFACT
    manifest_file = out / MANIFEST
    checksum_file = out / (ARTIFACT + ".sha256")
    archive_data = archive_file.read_bytes()
    manifest = json.loads(manifest_file.read_text(encoding="utf-8"))
    if manifest.get("bootable") is not False or manifest.get("flashable") is not False:
        raise ValueError("Invalid packaging safety markers")
    if manifest.get("archive") != {
        "file": ARTIFACT,
        "sha256": sha256(archive_data),
        "bytes": len(archive_data),
    }:
        raise ValueError("Archive checksum or length mismatch")
    if checksum_file.read_text(encoding="ascii") != "{}  {}\n".format(sha256(archive_data), ARTIFACT):
        raise ValueError("SHA256 sidecar mismatch")
    with tarfile.open(fileobj=io.BytesIO(archive_data), mode="r:gz") as tar:
        found = []
        for m in tar:
            if not m.isfile() or m.issym() or m.islnk() or m.uid or m.gid:
                raise ValueError("Unsafe archive entry")
            if m.name.startswith("/") or ".." in m.name.split("/") or m.name not in OVERLAY_FILES:
                raise ValueError("Unexpected archive path: " + m.name)
            if m.mtime != 0:
                raise ValueError("Non-reproducible tar timestamp")
            data = tar.extractfile(m).read()
            if m.name in ("usr/bin/hgfw-canary", "usr/bin/hgfw"):
                check_canary(data)
            found.append({"path": m.name, "bytes": len(data), "sha256": sha256(data), "mode": oct(m.mode)})
    if found != manifest.get("files"):
        raise ValueError("Archive contents differ from manifest")
    if len(found) != len(OVERLAY_FILES):
        raise ValueError("Missing overlay files")
    print("VERIFIED", len(found), "files; SHA256", sha256(archive_data))
    return True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("build", "verify"))
    parser.add_argument("--output-dir", help="Separate host output directory, never router storage")
    args = parser.parse_args()
    if args.action == "build":
        build(args.output_dir)
    else:
        verify(args.output_dir)


if __name__ == "__main__":
    main()

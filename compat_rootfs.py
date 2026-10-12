#!/usr/bin/env python3
"""Build a separate NONBOOTABLE HGFW compatibility development rootfs.

Uses exactly one verified stock binary (static ARM BusyBox) from the user's
preserved firmware extraction. No factory settings, services or device nodes.
This rootfs archive must never be flashed directly.
"""
import gzip
import os
import io
import json
from pathlib import Path
import tarfile

import fwkit

ROOT = fwkit.ROOT
STOCK_BUSYBOX = Path(os.environ.get("HGFW_STOCK_BUSYBOX", str(ROOT / "private" / "stock-busybox"))).expanduser()
ARCHIVE_NAME = "hgfw-compat-rootfs-armv7.tar.gz"
MANIFEST_NAME = "manifest-compat.json"
SYMLINKS = ("sh", "ls", "cat", "awk", "grep", "sed", "tr", "cut", "uname")
DIRS = ("bin", "dev", "etc", "etc/hgfw", "proc", "run", "sys", "tmp", "usr", "usr/bin", "usr/sbin")


def validate_busybox(data):
    fwkit.check_canary(data)
    if len(data) < 50_000 or len(data) > 5_000_000:
        raise ValueError("Unexpected extracted stock BusyBox size")
    # The existing stock BusyBox is *static*. Validate in WSL with readelf too.
    return True


def make_archive():
    if not STOCK_BUSYBOX.is_file():
        raise FileNotFoundError("Set HGFW_STOCK_BUSYBOX to your own extracted, authorized ARMv7 static BusyBox binary")
    bb = STOCK_BUSYBOX.read_bytes()
    validate_busybox(bb)
    entries = [("bin/busybox", bb, 0o755)]
    entries.extend(fwkit.members())
    items = {}
    for name, data, mode in entries:
        items[name] = ("file", data, mode)
    for name in SYMLINKS:
        items["bin/" + name] = ("link", "busybox", 0o777)
    for name in DIRS:
        items[name] = ("dir", None, 0o755)

    memory = io.BytesIO()
    with tarfile.open(fileobj=memory, mode="w", format=tarfile.USTAR_FORMAT) as tf:
        for name, (kind, data, mode) in sorted(items.items()):
            ti = tarfile.TarInfo(name)
            ti.mtime = ti.uid = ti.gid = 0
            ti.uname = ti.gname = "root"
            ti.mode = mode
            if kind == "file":
                ti.type = tarfile.REGTYPE
                ti.size = len(data)
                tf.addfile(ti, io.BytesIO(data))
            elif kind == "link":
                ti.type = tarfile.SYMTYPE
                ti.linkname = data
                tf.addfile(ti)
            else:
                ti.type = tarfile.DIRTYPE
                tf.addfile(ti)

    output = io.BytesIO()
    with gzip.GzipFile(filename="", fileobj=output, mode="wb", mtime=0, compresslevel=9) as f:
        f.write(memory.getvalue())
    return output.getvalue(), items


def manifest_for(archive, items):
    result = []
    for name, (kind, value, mode) in sorted(items.items()):
        e = {"path": name, "type": kind, "mode": oct(mode)}
        if kind == "file":
            e.update(bytes=len(value), sha256=fwkit.sha256(value))
        if kind == "link":
            e["target"] = value
        result.append(e)
    return {
        "schema": 1,
        "name": "HGFW compatibility research rootfs",
        "stage": "experimental-unbooted",
        "bootable": False,
        "flashable": False,
        "contains_one_stock_static_binary": True,
        "stock_file": "user-supplied original BusyBox; not included in public repository",
        "does_not_contain": ["calibration", "bootloader", "wifi driver", "ISP configs", "password database"],
        "archive": {"file": ARCHIVE_NAME, "bytes": len(archive), "sha256": fwkit.sha256(archive)},
        "entries": result,
    }


def build(out_dir=None):
    out = Path(out_dir) if out_dir else ROOT / "dist"
    data, items = make_archive()
    manifest = manifest_for(data, items)
    out.mkdir(parents=True, exist_ok=True)
    (out / ARCHIVE_NAME).write_bytes(data)
    (out / MANIFEST_NAME).write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print("BUILT", out / ARCHIVE_NAME, "({} bytes, {} entries)".format(len(data), len(items)))
    return manifest


def verify(out_dir=None):
    out = Path(out_dir) if out_dir else ROOT / "dist"
    b = (out / ARCHIVE_NAME).read_bytes()
    m = json.loads((out / MANIFEST_NAME).read_text(encoding="utf-8"))
    if m["bootable"] or m["flashable"]:
        raise ValueError("Invalid safety markers")
    if m["archive"] != {"file": ARCHIVE_NAME, "bytes": len(b), "sha256": fwkit.sha256(b)}:
        raise ValueError("Archive mismatch")
    if m["entries"] != manifest_for(*make_archive())["entries"]:
        raise ValueError("Unexpected entry inventory")
    seen = []
    with tarfile.open(fileobj=io.BytesIO(b), mode="r:gz") as tf:
        for member in tf:
            found = {"path": member.name, "mode": oct(member.mode)}
            if member.mtime or member.uid or member.gid:
                raise ValueError("Unsafe metadata")
            if member.name.startswith("/") or ".." in member.name.split("/"):
                raise ValueError("Unsafe path")
            if member.isfile():
                found["type"] = "file"
                data = tf.extractfile(member).read()
                found["bytes"] = len(data)
                found["sha256"] = fwkit.sha256(data)
                if member.name == "bin/busybox":
                    validate_busybox(data)
            elif member.issym() and member.linkname == "busybox" and member.name.startswith("bin/"):
                found["type"] = "link"
                found["target"] = member.linkname
            elif member.isdir():
                found["type"] = "dir"
            else:
                raise ValueError("Unapproved special tar entry")
            seen.append(found)
    if seen != m["entries"]:
        raise ValueError("Content mismatch")
    if "bin/busybox" not in [x["path"] for x in seen]:
        raise ValueError("Missing BusyBox")
    print("VERIFIED", len(seen), "entries; SHA256", fwkit.sha256(b))
    return True


if __name__ == "__main__":
    import argparse
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("action", choices=["build", "verify"])
    p.add_argument("--output-dir")
    args = p.parse_args()
    (build if args.action == "build" else verify)(args.output_dir)

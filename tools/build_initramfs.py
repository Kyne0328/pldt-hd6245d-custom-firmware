#!/usr/bin/env python3
"""Build a deterministic HGFW *experimental initramfs* for offline research.

No kernel, bootloader, vendor NAND layout, private factory configuration,
flash/upgrade API, network autostart or router modification is included.
A stock ARM BusyBox from the user's authorized extraction stays PRIVATE and
must NEVER be committed. The resulting archive is NOT an upgrade image.
"""
import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path
import struct

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_BUSYBOX = ROOT.parent / "analysis/extracted_mtd0/1080062015/rootfs_ubifs/bin/busybox"
DEFAULT_HGFW = ROOT / "build/hgfw-armv7-next"
NAME = "hgfw-experimental-initramfs-armv7.cpio.gz"
META = "hgfw-experimental-initramfs-manifest.json"

INIT = b"""#!/bin/sh
# Offline research initramfs. Requires a bootloader/kernel path not yet verified.
# No interface, GPON, Wi-Fi, DHCP, DNS, firewall or flash action is configured.
export PATH=/bin:/sbin:/usr/bin:/usr/sbin
/bin/mount -t proc proc /proc 2>/dev/null || :
/bin/mount -t sysfs sysfs /sys 2>/dev/null || :
/bin/mount -t devtmpfs devtmpfs /dev 2>/dev/null || :
/bin/mkdir -p /dev/pts
/bin/mount -t devpts devpts /dev/pts 2>/dev/null || :
echo 'HGFW EXPERIMENTAL INITRAMFS: NO HARDWARE INITIALIZATION OR FLASH ACCESS'
echo 'HGFW shell only; no network services are started.'
exec /bin/sh </dev/console >/dev/console 2>&1
"""

DIRS = (
    "bin", "dev", "dev/pts", "etc", "etc/hgfw", "proc", "run", "sbin",
    "sys", "tmp", "usr", "usr/bin",
)
LINKS = {
    "bin/sh": "busybox",
    "bin/mount": "busybox",
    "bin/mkdir": "busybox",
    "sbin/mdev": "../bin/busybox",
}
# Real device nodes are encoded as CPIO metadata only. No host mknod().
NODES = {
    "dev/console": (5, 1, 0o600),
    "dev/null": (1, 3, 0o666),
    "dev/zero": (1, 5, 0o666),
}
FILE = 0o100000
DIR = 0o040000
SYMLINK = 0o120000
CHR = 0o020000


def sha(data):
    return hashlib.sha256(data).hexdigest()


def check_elf32_arm(data, label):
    if len(data) < 60 or data[:7] != b"\x7fELF\x01\x01\x01":
        raise ValueError(f"{label} is not an ELF32 little-endian binary")
    if struct.unpack_from("<H", data, 18)[0] != 40:
        raise ValueError(f"{label} is not ARM EABI")
    phoff = struct.unpack_from("<I", data, 28)[0]
    phentsize = struct.unpack_from("<H", data, 42)[0]
    phnum = struct.unpack_from("<H", data, 44)[0]
    if phentsize < 32 or phnum < 1 or phnum > 128 or phoff + phnum * phentsize > len(data):
        raise ValueError(f"{label} has invalid program headers")
    for idx in range(phnum):
        if struct.unpack_from("<I", data, phoff + idx * phentsize)[0] == 3:
            raise ValueError(f"{label} needs a dynamic ELF interpreter")


def contents(busybox, hgfw, services):
    bb = Path(busybox).read_bytes()
    go = Path(hgfw).read_bytes()
    check_elf32_arm(bb, "stock BusyBox")
    check_elf32_arm(go, "HGFW")
    if not 50000 < len(bb) < 5000000:
        raise ValueError("stock BusyBox unexpected size")
    if not 100000 < len(go) < 16000000:
        raise ValueError("HGFW unexpected size")
    config = Path(services).read_bytes()
    decoded = json.loads(config)
    if decoded != {"services": []}:
        raise ValueError("initramfs must have no enabled service definitions")
    result = {}
    for name in DIRS:
        result[name] = ("dir", DIR | 0o755, b"", 0, 0)
    for name, target in LINKS.items():
        result[name] = ("symlink", SYMLINK | 0o777, target.encode("ascii"), 0, 0)
    for name, (major, minor, perms) in NODES.items():
        result[name] = ("char", CHR | perms, b"", major, minor)
    result["bin/busybox"] = ("file", FILE | 0o755, bb, 0, 0)
    result["usr/bin/hgfw"] = ("file", FILE | 0o755, go, 0, 0)
    result["etc/hgfw/services.json"] = ("file", FILE | 0o644, config, 0, 0)
    result["init"] = ("file", FILE | 0o755, INIT, 0, 0)
    return result


def newc_record(name, mode, value, inode, rmajor=0, rminor=0):
    if name.startswith("/") or ".." in name.split("/") or not name.isascii():
        raise ValueError("unsafe archive entry")
    name_b = name.encode("ascii") + b"\0"
    fields = (inode, mode, 0, 0, 2 if mode & 0o170000 == DIR else 1,
              0, len(value), 0, 0, rmajor, rminor, len(name_b), 0)
    header = b"070701" + b"".join(f"{n:08x}".encode() for n in fields)
    if len(header) != 110:
        raise AssertionError("bad newc header size")
    output = header + name_b
    output += b"\0" * (-len(output) % 4)
    output += value
    output += b"\0" * (-len(value) % 4)
    return output


def make_newc(entries):
    raw = io.BytesIO()
    for inode, name in enumerate(sorted(entries), start=1):
        kind, mode, value, major, minor = entries[name]
        raw.write(newc_record(name, mode, value, inode, major, minor))
    raw.write(newc_record("TRAILER!!!", 0, b"", len(entries) + 1))
    return raw.getvalue()


def generate(busybox, hgfw, services, output):
    entries = contents(busybox, hgfw, services)
    content = make_newc(entries)
    zipped = io.BytesIO()
    with gzip.GzipFile(filename="", mode="wb", fileobj=zipped, mtime=0, compresslevel=9) as comp:
        comp.write(content)
    archive = zipped.getvalue()
    out = Path(output)
    out.mkdir(parents=True, exist_ok=True)
    rows = []
    for name in sorted(entries):
        kind, mode, body, major, minor = entries[name]
        row = {"path": name, "kind": kind, "mode": oct(mode), "bytes": len(body)}
        if kind == "file":
            row["sha256"] = sha(body)
        if kind == "symlink":
            row["target"] = body.decode("ascii")
        if kind == "char":
            row["major"] = major
            row["minor"] = minor
        rows.append(row)
    manifest = {
        "format": "newc+gzip",
        "platform": "ARMv7 BCM96846 experiment",
        "stage": "kernel-boot-unverified",
        "flashable": False,
        "production_ready": False,
        "contains_private_vendor_busybox": True,
        "contains_kernel": False,
        "contains_board_drivers": False,
        "contains_calibration_or_factory_data": False,
        "starts_network_services": False,
        "entrypoint": "/init",
        "requires_verified_ram_boot_method": True,
        "archive": {"file": NAME, "bytes": len(archive), "sha256": sha(archive)},
        "entries": rows,
    }
    (out / NAME).write_bytes(archive)
    (out / META).write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(f"BUILT {out / NAME} ({len(archive)} bytes, {len(entries)} entries)")
    print(f"SHA256 {sha(archive)}")
    return manifest


def inspect_archive(archive):
    import gzip as gz
    b = gz.decompress(archive)
    entries = []
    p = 0
    seen = set()
    while p + 110 <= len(b):
        if b[p:p+6] != b"070701":
            raise ValueError("invalid newc magic")
        values = [int(b[p+6+i*8:p+14+i*8], 16) for i in range(13)]
        inode, mode, uid, gid, nlink, mtime, sz, maj, minr, rmaj, rminr, namesize, checksum = values
        if namesize < 2 or namesize > 4096 or mtime != 0 or uid != 0 or gid != 0 or checksum:
            raise ValueError("invalid newc metadata")
        name_start = p + 110
        name_end = name_start + namesize
        if name_end > len(b) or b[name_end-1] != 0:
            raise ValueError("bad newc name")
        name = b[name_start:name_end-1].decode("ascii")
        p = (name_end + 3) & ~3
        value = b[p:p+sz]
        if len(value) != sz:
            raise ValueError("truncated newc payload")
        p = (p+sz+3) & ~3
        if name == "TRAILER!!!":
            if any(b[p:]):
                raise ValueError("unexpected trailing cpio bytes")
            return entries
        if name in seen or name.startswith("/") or ".." in name.split("/"):
            raise ValueError("duplicate or unsafe newc entry")
        seen.add(name)
        kind = {FILE: "file", DIR: "dir", SYMLINK: "symlink", CHR: "char"}.get(mode & 0o170000)
        if not kind:
            raise ValueError("unexpected filesystem entry type")
        row = {"path": name, "kind": kind, "mode": oct(mode), "bytes": sz}
        if kind == "file":
            row["sha256"] = sha(value)
        if kind == "symlink":
            row["target"] = value.decode("ascii")
        if kind == "char":
            row["major"] = rmaj
            row["minor"] = rminr
        entries.append(row)
    raise ValueError("missing newc trailer")


def verify(output):
    out = Path(output)
    manifest = json.loads((out / META).read_text(encoding="utf-8"))
    data = (out / NAME).read_bytes()
    if manifest["flashable"] or manifest["production_ready"] or manifest["contains_kernel"]:
        raise ValueError("unsafe claimed status")
    if manifest["archive"] != {"file": NAME, "bytes": len(data), "sha256": sha(data)}:
        raise ValueError("archive hash mismatch")
    if inspect_archive(data) != manifest["entries"]:
        raise ValueError("initramfs entries differ from manifest")
    print("VERIFIED", len(manifest["entries"]), "entries", sha(data))
    return True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("build", "verify"))
    parser.add_argument("--busybox", type=Path, default=DEFAULT_BUSYBOX)
    parser.add_argument("--hgfw", type=Path, default=DEFAULT_HGFW)
    parser.add_argument("--services", type=Path, default=ROOT / "config/services.json")
    parser.add_argument("--output-dir", type=Path, default=ROOT / "dist/private-initramfs")
    options = parser.parse_args()
    if options.action == "build":
        generate(options.busybox, options.hgfw, options.services, options.output_dir)
    else:
        verify(options.output_dir)


if __name__ == "__main__":
    main()

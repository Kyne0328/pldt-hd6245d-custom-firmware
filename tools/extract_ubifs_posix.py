#!/usr/bin/env python3
"""Re-extract a preserved UBIFS volume on Linux with permission *bits* intact.

Requires an independently installed ubi_reader, lzallright, zstandard and
cryptography. Runs as a normal, non-root developer user and NEVER restores
ownership, setuid/setgid privilege or device nodes. Avoid executing as root.
This is private forensic staging, not a bootable filesystem or flash image.
"""
import argparse
import os
from pathlib import Path
import runpy
import stat
import sys


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("volume", type=Path)
    p.add_argument("--out", required=True, type=Path)
    p.add_argument("--leb-size", type=int, default=126976)
    p.add_argument("--reader-script", required=True, type=Path)
    p.add_argument("--require-busybox", action="store_true", help="For the stock rootfs volume only")
    args = p.parse_args()
    if os.geteuid() == 0:
        raise SystemExit("refusing root execution: extract as unprivileged developer")
    if args.leb_size < 4096 or args.leb_size > 1048576:
        raise SystemExit("invalid logical eraseblock size")
    source = args.volume.resolve(strict=True)
    dest = args.out.absolute()
    if dest.exists() or dest.is_symlink() or source.is_dir():
        raise SystemExit("destination exists or input is not a volume file")
    if not args.reader_script.is_file():
        raise SystemExit("missing trusted offline ubi_reader entrypoint")

    import ubireader.ubifs.output as output

    def restore_nonprivileged_mode(path, inode):
        # No chown, setuid, setgid, sticky or symlink following. Restore only
        # POSIX read/write/execute bits recorded in the original UBIFS inode.
        if not os.path.islink(path):
            os.chmod(path, stat.S_IMODE(inode["ino"].mode) & 0o777)

    output._set_file_perms = restore_nonprivileged_mode
    saved = sys.argv[:]
    try:
        sys.argv = [
            str(args.reader_script), "-k", "-e", str(args.leb_size),
            "-o", str(dest), str(source),
        ]
        runpy.run_path(str(args.reader_script), run_name="__main__")
    finally:
        sys.argv = saved
    if not dest.is_dir() or not any(dest.iterdir()):
        raise SystemExit("extraction incomplete: output is missing or empty")
    if args.require_busybox and not (dest / "bin/busybox").is_file():
        raise SystemExit("rootfs extraction incomplete: no BusyBox recovered")
    # Read metadata only. Do not print confidential contents or file paths.
    reg = sum(1 for x in dest.rglob("*") if x.is_file() and not x.is_symlink())
    links = sum(1 for x in dest.rglob("*") if x.is_symlink())
    print(f"UBIFS_POSIX_REEXTRACTION_OK files={reg} symlinks={links}")
    print("OWNERSHIP_RESTORED=false")
    print("FLASH_READY=false")


if __name__ == "__main__":
    main()

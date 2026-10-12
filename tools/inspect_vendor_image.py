#!/usr/bin/env python3
"""Read-only inspection of FiberHome HG6245D firmware boot-file containers.

Original source binaries remain untouched. Never extracts private provisioning
or writes vendor images to MTD. Inspects specifically the FDT/CFE/kernel record
format established by comparison with RP2613's filestruct_full.bin UBI volume.
"""
import argparse
import hashlib
import json
from pathlib import Path
import re
import struct

MAGIC_FDT = b"\xd0\x0d\xfe\xed"
DTB_NAME = b"96846.dtb\x00"
MAX_BYTES = 100 * 1024 * 1024
MAX_RECORD = 16 * 1024 * 1024
ALLOWED_NAMES = ("96846.dtb", "cferam.000", "vmlinux.lz", "image_ident", "image_version")


def checksum(data):
    return hashlib.sha256(data).hexdigest()


def read_file(path):
    p = Path(path)
    size = p.stat().st_size
    if not p.is_file() or size > MAX_BYTES or size < 64:
        raise ValueError("invalid or oversized input")
    return p.read_bytes()


def find_container(raw):
    # A valid first record is 16-byte header followed by the expected filename.
    # Its validated payload must start with flattened device-tree magic.
    marker = 0
    while True:
        marker = raw.find(DTB_NAME, marker)
        if marker < 0:
            raise ValueError("no supported filestruct container header found")
        start = marker - 16
        if start >= 0 and raw[marker + len(DTB_NAME):marker + len(DTB_NAME) + 4] == MAGIC_FDT:
            try:
                parsed = parse_at(raw, start)
                if parsed and parsed[0]["name"] == "96846.dtb":
                    return start, parsed
            except ValueError:
                pass
        marker += 1


def parse_at(raw, offset):
    if offset < 0 or offset >= len(raw):
        raise ValueError("invalid container offset")
    records = []
    pos = offset
    for index, expected in enumerate(ALLOWED_NAMES):
        if pos + 20 > len(raw):
            raise ValueError("truncated record header")
        total, kind, declared, opaque_field = struct.unpack_from(">IIII", raw, pos)
        if total < 20 or total > MAX_RECORD or total > len(raw) - pos:
            raise ValueError("invalid record length")
        end_name = raw.find(b"\x00", pos + 16, min(pos + 96, pos + total))
        if end_name < 0:
            raise ValueError("unterminated record name")
        name = raw[pos + 16:end_name].decode("ascii")
        if name != expected:
            raise ValueError("unexpected record order or name")
        if kind != (18 if index == 0 else 19 if index in (1, 2) else 20 if index == 3 else 22):
            raise ValueError("unexpected record type")
        start_payload = end_name + 1
        if start_payload + declared != pos + total:
            raise ValueError("record length mismatch")
        value = raw[start_payload:pos + total]
        if index == 0:
            if not valid_fdt(value):
                raise ValueError("invalid embedded FDT")
        if pos + total + 4 > len(raw):
            raise ValueError("missing record trailer")
        # Each record has a 32-bit trailer, algorithm not yet verified.
        trailer = struct.unpack_from(">I", raw, pos + total)[0]
        records.append({
            "name": name,
            "type": kind,
            "offset": pos,
            "payloadOffset": start_payload,
            "payloadBytes": declared,
            "payloadSHA256": checksum(value),
            "opaqueHeaderField": f"{opaque_field:08x}",
            "opaqueTrailerField": f"{trailer:08x}",
        })
        pos += total + 4
    return records


def valid_fdt(blob):
    if len(blob) < 40 or blob[:4] != MAGIC_FDT:
        return False
    (magic, size, off_struct, off_string, off_mem, version, last_compatible,
     cpu, str_size, struct_size) = struct.unpack_from(">10I", blob)
    return (size == len(blob) and 40 <= off_mem < size and
            40 <= off_struct < size and 40 <= off_string < size and
            off_struct + struct_size <= size and off_string + str_size <= size and
            16 <= version <= 17 and last_compatible <= version)


def board_headers(raw):
    # Four NUL-terminated, fixed-width WKE hardware identifiers in vendor update.
    if len(raw) < 100:
        return None
    segments = [raw[i:i + 16].split(b"\0", 1)[0] for i in (0, 16, 32, 48)]
    if not all(re.fullmatch(rb"WKE[0-9A-Za-z.]{8,20}", x) for x in segments):
        return None
    version_marker = raw[80:96].split(b"\0", 1)[0].decode("ascii", "replace")
    return {"allowedBoards": [x.decode("ascii") for x in segments],
            "versionMarker": version_marker}


def analyze(path, reference=None):
    data = read_file(path)
    offset, records = find_container(data)
    report = {
        "name": Path(path).name,
        "bytes": len(data),
        "sha256": checksum(data),
        "vendorUpdateBoardHeader": board_headers(data),
        "filestructOffset": offset,
        "filestructRecords": records,
        "bootableFirmwareVerified": False,
        "nandFlashable": False,
        "checksumsUnderstood": False,
        "notes": [
            "Original images are never changed or run by this analyzer.",
            "The 32-bit opaque fields are not assumed to be verified checksums.",
            "Matching board IDs or kernel metadata cannot establish recovery or compatibility.",
        ],
    }
    if reference is not None:
        other = read_file(reference)
        ref_offset, ref_records = find_container(other)
        by_name = {r["name"]: r for r in ref_records}
        report["reference"] = {
            "name": Path(reference).name,
            "filestructOffset": ref_offset,
            "identicalPayloads": {
                r["name"]: r["payloadSHA256"] == by_name[r["name"]]["payloadSHA256"]
                for r in records
            },
        }
    return report


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("image", type=Path)
    p.add_argument("--reference", type=Path)
    p.add_argument("--output", type=Path)
    args = p.parse_args()
    data = json.dumps(analyze(args.image, args.reference), indent=2, sort_keys=True) + "\n"
    if args.output is not None:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(data, encoding="utf-8")
        print("WROTE", args.output)
    else:
        print(data, end="")


if __name__ == "__main__":
    main()

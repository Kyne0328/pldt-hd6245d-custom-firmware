"""Read-only FiberHome update image parser fixtures."""
from pathlib import Path
import struct
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from tools import inspect_vendor_image as vi


def fdt():
    data = bytearray(128)
    struct.pack_into(">10I", data, 0,
                     0xd00dfeed, 128, 56, 80, 40, 17, 16, 0, 8, 20)
    data[56:60] = b"\x00\x00\x00\x09"
    return bytes(data)


def record(name, kind, content):
    nb = name.encode() + b"\0"
    total = 16 + len(nb) + len(content)
    return struct.pack(">4I", total, kind, len(content), 0x12345678) + nb + content + struct.pack(">I", 0x87654321)


def filestruct():
    entries = [
        ("96846.dtb", 18, fdt()),
        ("cferam.000", 19, b"bootloader-stub"),
        ("vmlinux.lz", 19, b"kernel-stub"),
        ("image_ident", 20, b"board-stub"),
        ("image_version", 22, b"RP0000-stub"),
    ]
    return b"".join(record(*row) for row in entries)


class VendorImageTests(unittest.TestCase):
    def test_parses_only_valid_header_and_known_entries(self):
        fst = filestruct()
        image = b"unused-header" * 16 + fst + b"postfix"
        start, entries = vi.find_container(image)
        self.assertEqual(start, 13 * 16)
        self.assertEqual([r["name"] for r in entries], list(vi.ALLOWED_NAMES))
        self.assertEqual(entries[0]["payloadBytes"], len(fdt()))
        self.assertFalse(vi.board_headers(image))

    def test_detects_tampered_fdt_and_record_length(self):
        corrupt = bytearray(filestruct())
        corrupt[16 + len("96846.dtb") + 1] ^= 1
        with self.assertRaises(ValueError):
            vi.find_container(bytes(corrupt))
        corrupt = bytearray(filestruct())
        corrupt[0:4] = b"\xff" * 4
        with self.assertRaises(ValueError):
            vi.parse_at(bytes(corrupt), 0)

    def test_board_allowlist_is_explicit(self):
        raw = bytearray(256)
        for i, value in enumerate(["WKE2.094.278A01", "WKE2.094.277A01", "WKE2.094.288A01", "WKE2.094.290A01"]):
            raw[i * 16:i * 16 + len(value)] = value.encode()
        raw[80:89] = b"SIPRP2725"
        meta = vi.board_headers(raw)
        self.assertIn("WKE2.094.277A01", meta["allowedBoards"])
        self.assertEqual(meta["versionMarker"], "SIPRP2725")

    def test_non_destructive_report_and_reference(self):
        with tempfile.TemporaryDirectory() as td:
            p = Path(td) / "vendor.bin"
            r = Path(td) / "stock.img"
            # Unrecognized vendor header, but authentic-looking named records.
            p.write_bytes(b"x" * 256 + filestruct())
            r.write_bytes(filestruct())
            out = vi.analyze(p, r)
            self.assertFalse(out["nandFlashable"])
            self.assertFalse(out["bootableFirmwareVerified"])
            self.assertTrue(all(out["reference"]["identicalPayloads"].values()))
            self.assertEqual(p.read_bytes(), b"x" * 256 + filestruct())


if __name__ == "__main__":
    unittest.main()

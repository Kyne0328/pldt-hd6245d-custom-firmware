"""HGFW offline initramfs builder tests (no router or vendor binaries required)."""
import gzip
import json
from pathlib import Path
import struct
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from tools import build_initramfs as ir


def elf_fixture(length):
    b = bytearray(length)
    b[:7] = b"\x7fELF\x01\x01\x01"
    struct.pack_into("<H", b, 18, 40)  # ARM
    struct.pack_into("<I", b, 28, 52)  # program headers
    struct.pack_into("<H", b, 42, 32)
    struct.pack_into("<H", b, 44, 1)
    struct.pack_into("<I", b, 52, 1)  # LOAD, not INTERP
    return bytes(b)


class InitramfsTests(unittest.TestCase):
    def inputs(self, location):
        folder = Path(location)
        bb, hgfw, cfg = (folder / "busybox", folder / "hgfw", folder / "services.json")
        bb.write_bytes(elf_fixture(70000))
        hgfw.write_bytes(elf_fixture(130000))
        cfg.write_text('{"services":[]}', encoding="utf-8")
        return bb, hgfw, cfg

    def test_reproducible_cpio_archive(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            bb, hgfw, cfg = self.inputs(td)
            a, b = root / "a", root / "b"
            one = ir.generate(bb, hgfw, cfg, a)
            two = ir.generate(bb, hgfw, cfg, b)
            self.assertEqual(one, two)
            self.assertEqual((a / ir.NAME).read_bytes(), (b / ir.NAME).read_bytes())
            self.assertTrue(ir.verify(a))
            self.assertFalse(one["flashable"])
            self.assertFalse(one["starts_network_services"])
            self.assertIn("init", [r["path"] for r in one["entries"]])
            self.assertIn("dev/console", [r["path"] for r in one["entries"]])
            self.assertIn("usr/bin/hgfw", [r["path"] for r in one["entries"]])
            self.assertIn("bin/busybox", [r["path"] for r in one["entries"]])
            self.assertNotIn("etc/shadow", [r["path"] for r in one["entries"]])
            self.assertEqual(gzip.decompress((a / ir.NAME).read_bytes())[:6], b"070701")

    def test_rejects_configured_network_services(self):
        with tempfile.TemporaryDirectory() as td:
            bb, exe, cfg = self.inputs(td)
            cfg.write_text('{"services":[{"name":"dhcp","enabled":true}]}', encoding="utf-8")
            with self.assertRaises(ValueError):
                ir.generate(bb, exe, cfg, Path(td) / "out")

    def test_rejects_dynamic_binary(self):
        data = bytearray(elf_fixture(80000))
        struct.pack_into("<I", data, 52, 3)  # INTERP
        with self.assertRaises(ValueError):
            ir.check_elf32_arm(bytes(data), "test")

    def test_detects_tamper(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            bb, exe, cfg = self.inputs(td)
            out = root / "out"
            ir.generate(bb, exe, cfg, out)
            original = (out / ir.NAME).read_bytes()
            (out / ir.NAME).write_bytes(original + b"tampered")
            with self.assertRaises(ValueError):
                ir.verify(out)
            (out / ir.NAME).write_bytes(original)
            mf = out / ir.META
            changed = json.loads(mf.read_text())
            changed["archive"]["sha256"] = "0" * 64
            mf.write_text(json.dumps(changed))
            with self.assertRaises(ValueError):
                ir.verify(out)

    def test_rejects_bad_cpio_member(self):
        with self.assertRaises(ValueError):
            ir.newc_record("../escape", 0o100644, b"", 1)


if __name__ == "__main__":
    unittest.main()

"""Private RP2613 rootfs offline integration contract tests."""
from pathlib import Path
import json
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from tools import stage_hybrid_rootfs as hy


class HybridRootfsTests(unittest.TestCase):
    def fixture(self, base):
        source = base / "stock"
        for d in ("bin", "etc", "etc/init.d", "fhconf", "var/log", "lib/modules/4.1.45/extra"):
            (source / d).mkdir(parents=True, exist_ok=True)
        (source / "bin/busybox").write_bytes(b"stock-busybox")
        (source / "etc/init.d/rcS").write_text("#!/bin/sh\necho original\n")
        (source / "fhconf/secret.cfg").write_text("DO_NOT_PACKAGE")
        (source / "var/log/client.log").write_text("PERSONAL_DATA")
        (source / "lib/modules/4.1.45/extra/wl.ko").write_bytes(b"private-module")
        (source / "etc/shadow").write_text("password-hash")
        (source / "bin/sh").symlink_to("busybox")
        hgfw = base / "hgfw"
        hgfw.write_bytes(b"\x7fELF\x01\x01\x01" + b"\0" * 500100)
        return source, hgfw

    def test_staging_only_and_reproducible_inventory(self):
        with tempfile.TemporaryDirectory() as td:
            d = Path(td)
            source, exe = self.fixture(d)
            output = d / "candidate"
            m = hy.stage(source, exe, output)
            self.assertFalse(m["bootable"])
            self.assertFalse(m["flashable"])
            self.assertFalse(m["automaticallyStartsHGFW"])
            self.assertFalse((output / "rootfs/fhconf").exists())
            self.assertFalse((output / "rootfs/var/log").exists())
            self.assertFalse((output / "rootfs/etc/shadow").exists())
            self.assertTrue((output / "rootfs/lib/modules/4.1.45/extra/wl.ko").exists())
            self.assertTrue((output / "rootfs/bin/sh").is_symlink())
            self.assertTrue((output / "rootfs/usr/bin/hgfw").exists())
            self.assertEqual((source / "fhconf/secret.cfg").read_text(), "DO_NOT_PACKAGE")
            self.assertTrue(hy.verify(output))
            with self.assertRaises(FileExistsError):
                hy.stage(source, exe, output)
            (output / "rootfs/bin/busybox").write_bytes(b"tampered")
            with self.assertRaises(ValueError):
                hy.verify(output)

    def test_disallows_nested_sensitive_directories(self):
        self.assertTrue(hy.denied(Path("etc/wlan/key")))
        self.assertTrue(hy.denied(Path("preconf")))
        self.assertFalse(hy.denied(Path("lib/modules/wl.ko")))

    def test_rejects_overlapping_source_and_output(self):
        with tempfile.TemporaryDirectory() as td:
            d = Path(td)
            source, exe = self.fixture(d)
            with self.assertRaises(ValueError):
                hy.stage(source, exe, source / "copied")


if __name__ == "__main__":
    unittest.main()

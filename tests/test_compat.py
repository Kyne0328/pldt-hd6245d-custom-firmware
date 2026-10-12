"""Compatibility research rootfs packaging tests."""
from pathlib import Path
import json
import sys
import tarfile
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import compat_rootfs


class CompatTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if not compat_rootfs.STOCK_BUSYBOX.exists():
            raise unittest.SkipTest("Original stock BusyBox missing")

    def test_manifest_and_tar_verify(self):
        with tempfile.TemporaryDirectory() as td:
            m = compat_rootfs.build(td)
            self.assertFalse(m["bootable"])
            self.assertFalse(m["flashable"])
            self.assertTrue(compat_rootfs.verify(td))
            with tarfile.open(Path(td) / compat_rootfs.ARCHIVE_NAME, "r:gz") as archive:
                self.assertTrue(archive.getmember("bin/sh").issym())
                self.assertEqual(archive.getmember("bin/sh").linkname, "busybox")
                self.assertTrue(archive.getmember("bin/busybox").isfile())
                self.assertIsNone(archive.getmember("bin/busybox").pax_headers.get("path"))

    def test_reproducibility(self):
        with tempfile.TemporaryDirectory() as a, tempfile.TemporaryDirectory() as b:
            compat_rootfs.build(a)
            compat_rootfs.build(b)
            self.assertEqual((Path(a) / compat_rootfs.ARCHIVE_NAME).read_bytes(),
                             (Path(b) / compat_rootfs.ARCHIVE_NAME).read_bytes())

    def test_no_private_vendor_settings(self):
        with tempfile.TemporaryDirectory() as td:
            compat_rootfs.build(td)
            with tarfile.open(Path(td) / compat_rootfs.ARCHIVE_NAME, "r:gz") as archive:
                names = archive.getnames()
                for forbidden in ("etc/passwd", "etc/shadow", "fhconf", "nvram", "lib/modules", "etc/ppp"):
                    self.assertFalse(any(forbidden in name for name in names), forbidden)
                self.assertEqual(sum(name == "bin/busybox" for name in names), 1)


if __name__ == "__main__":
    unittest.main()

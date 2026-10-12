"""Private hybrid rootfs archive reproducibility and integrity tests."""
import io
import pathlib
import sys
import tarfile
import tempfile
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))
from tools import package_hybrid_rootfs as ph
from tools import stage_hybrid_rootfs as sh


class HybridArchiveTests(unittest.TestCase):
    def test_determinism_integrity_and_no_secret_files(self):
        with tempfile.TemporaryDirectory() as folder:
            t = pathlib.Path(folder)
            root = t / "stock"
            for sub in ("bin", "lib/modules", "etc/init.d", "etc/rc3.d", "fhconf"):
                (root / sub).mkdir(parents=True, exist_ok=True)
            (root / "bin/busybox").write_bytes(b"busybox")
            (root / "lib/modules/wl.ko").write_bytes(b"private-binary")
            (root / "fhconf/credentials").write_text("secret")
            (root / "etc/init.d/mount-fs.sh").write_text("#!/bin/sh\n")
            (root / "etc/rc3.d/S25mount-fs").symlink_to("../init.d/mount-fs.sh")
            gh = t / "hgfw"
            gh.write_bytes(b"\x7fELF\x01\x01\x01" + b"\0" * 500120)
            staging = t / "staging"
            sh.stage(root, gh, staging, mode_bits_from_ubifs=True)
            a, b = t / "buildA", t / "buildB"
            one = ph.build(staging / "rootfs", a, all_root_owners_verified=True)
            two = ph.build(staging / "rootfs", b, all_root_owners_verified=True)
            self.assertEqual(one, two)
            self.assertEqual((a / ph.NAME).read_bytes(), (b / ph.NAME).read_bytes())
            self.assertTrue(ph.verify(a))
            self.assertFalse(one["flashable"])
            self.assertFalse(one["bootable"])
            paths = {x["path"] for x in one["members"]}
            self.assertNotIn("fhconf/credentials", paths)
            self.assertIn("etc/rc3.d/S25mount-fs", paths)
            with tarfile.open(fileobj=io.BytesIO((a / ph.NAME).read_bytes()), mode="r:gz") as arc:
                self.assertTrue(arc.getmember("etc/rc3.d/S25mount-fs").issym())
                self.assertEqual(arc.getmember("bin/busybox").uid, 0)
                self.assertEqual(arc.getmember("bin/busybox").gid, 0)
            payload = a / ph.NAME
            payload.write_bytes(payload.read_bytes() + b"tampered")
            with self.assertRaises(ValueError):
                ph.verify(a)

    def test_no_unverified_owner_source(self):
        with tempfile.TemporaryDirectory() as folder:
            with self.assertRaises(ValueError):
                ph.build(pathlib.Path(folder), pathlib.Path(folder) / "out")

    def test_no_unverified_mode_source(self):
        with tempfile.TemporaryDirectory() as folder:
            t = pathlib.Path(folder)
            (t / "rootfs").mkdir()
            (t / "manifest.json").write_text('{"bootable":false,"flashable":false,"factoryProvisioningIncluded":false,"modeBitsRestoredFromUBIFS":false,"files":[]}')
            with self.assertRaises(ValueError):
                ph.members_from_tree(t / "rootfs")


if __name__ == "__main__":
    unittest.main()

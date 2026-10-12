"""HGFW stage1 host-side release safety checks."""
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import sys
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
import fwkit


class BuildTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if not fwkit.CANARY.exists():
            raise unittest.SkipTest("Cross-build tools/build_arm.sh before packaging tests")

    def test_canary_machine_is_arm(self):
        fwkit.check_canary(fwkit.CANARY.read_bytes())

    def test_build_is_reproducible(self):
        with tempfile.TemporaryDirectory() as tmp1, tempfile.TemporaryDirectory() as tmp2:
            m1 = fwkit.build(tmp1)
            m2 = fwkit.build(tmp2)
            self.assertEqual(m1, m2)
            self.assertEqual((Path(tmp1) / fwkit.ARTIFACT).read_bytes(),
                             (Path(tmp2) / fwkit.ARTIFACT).read_bytes())
            self.assertTrue(fwkit.verify(tmp1))
            self.assertTrue(fwkit.verify(tmp2))
            self.assertFalse(m1["bootable"])
            self.assertFalse(m1["flashable"])
            self.assertFalse(m1["stock_files_copied"])

    def test_archive_tamper_is_detected(self):
        with tempfile.TemporaryDirectory() as tmp:
            fwkit.build(tmp)
            archive = Path(tmp) / fwkit.ARTIFACT
            archive.write_bytes(archive.read_bytes() + b"tampered")
            with self.assertRaises(ValueError):
                fwkit.verify(tmp)

    def test_manifest_tamper_is_detected(self):
        with tempfile.TemporaryDirectory() as tmp:
            fwkit.build(tmp)
            manifest = Path(tmp) / fwkit.MANIFEST
            data = json.loads(manifest.read_text(encoding="utf-8"))
            data["archive"]["sha256"] = "0" * 64
            manifest.write_text(json.dumps(data), encoding="utf-8")
            with self.assertRaises(ValueError):
                fwkit.verify(tmp)

    def test_overlay_paths_are_safe(self):
        entries = fwkit.members()
        self.assertEqual(len(entries), len(fwkit.OVERLAY_FILES))
        for name, data, mode in entries:
            self.assertNotIn("..", name.split("/"))
            self.assertFalse(name.startswith("/"))
            self.assertFalse(name.startswith("rootfsA"))
            self.assertFalse(name.startswith("fhconf"))
            self.assertIn(mode, (0o755, 0o644))
            self.assertGreater(len(data), 0)

    def test_no_stock_binary_in_package(self):
        with tempfile.TemporaryDirectory() as tmp:
            fwkit.build(tmp)
            with tarfile.open(Path(tmp) / fwkit.ARTIFACT, "r:gz") as tf:
                self.assertEqual(sorted(tf.getnames()), sorted(fwkit.OVERLAY_FILES))
                for name in tf.getnames():
                    self.assertNotIn("wl.ko", name)
                    self.assertNotIn("nvram", name)
                    self.assertNotIn("nas", name.lower())


if __name__ == "__main__":
    unittest.main()

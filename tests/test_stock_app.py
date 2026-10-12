"""Offline stock app staging: no provisioning and no automatic services."""
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from tools import stage_stock_app as app
from tools import package_hybrid_rootfs as archive

class AppVolumeTests(unittest.TestCase):
    def test_private_app_stage_reproducible_and_provisioning_excluded(self):
        with tempfile.TemporaryDirectory() as td:
            base=Path(td)
            src=base/"source"
            for d in ("fhshell","bin","lib","fhconf"):
                (src/d).mkdir(parents=True,exist_ok=True)
            (src/"fhshell/udevinit.sh").write_text("#!/bin/sh\n")
            (src/"bin/daemon").write_bytes(b"vendor-executable")
            (src/"lib/libvendor.so").write_bytes(b"vendor-library")
            (src/"bin/agent").symlink_to("daemon")
            (src/"fhconf/passwords").write_text("PRIVATE")
            staged=base/"staged"
            with self.assertRaises(ValueError):
                app.stage(src,staged)
            app.stage(src,staged,owners_verified=True)
            self.assertTrue(app.verify(staged))
            self.assertFalse((staged/"rootfs/fhconf").exists())
            self.assertTrue((staged/"rootfs/bin/agent").is_symlink())
            with self.assertRaises(FileExistsError):
                app.stage(src,staged,owners_verified=True)
            out=base/"out"
            archive.build(staged/"rootfs",out,all_root_owners_verified=True,profile="app")
            self.assertTrue(archive.verify(out,profile="app"))
            self.assertTrue((out / archive.APP_NAME).is_file())
            (staged/"rootfs/bin/daemon").write_bytes(b"tampered")
            with self.assertRaises(ValueError):
                app.verify(staged)

if __name__=="__main__": unittest.main()

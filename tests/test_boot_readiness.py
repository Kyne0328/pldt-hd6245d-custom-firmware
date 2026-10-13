"""Installation readiness must remain blocked even for valid component hashes."""
from pathlib import Path
import hashlib
import json
import sys
import tempfile
import unittest
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from tools import assess_boot_readiness as gate

class GateTests(unittest.TestCase):
    def test_valid_archives_still_fail_installation_gate(self):
        with tempfile.TemporaryDirectory() as td:
            base=Path(td)
            def make(folder,name,manifest):
                p=base/folder;p.mkdir()
                raw=b"offline-byte-fixture"
                (p/name).write_bytes(raw)
                (p/manifest).write_text(json.dumps({"flashable":False,"bootable":False,"archive":{
                   "file":name,"bytes":len(raw),"sha256":hashlib.sha256(raw).hexdigest()}}))
                return p
            init=make("init","hgfw-experimental-initramfs-armv7.cpio.gz","hgfw-experimental-initramfs-manifest.json")
            root=make("root","hgfw-rp2613-hybrid-rootfs-private.tar.gz","hgfw-rp2613-hybrid-rootfs-private.json")
            app=make("app","hgfw-rp2613-app-compat-private.tar.gz","hgfw-rp2613-app-compat-private.json")
            report=gate.assess(init,root,app)
            self.assertTrue(report["passedLocalArtifactIntegrity"])
            self.assertFalse(report["flashable"])
            self.assertFalse(report["installationPermitted"])
            self.assertEqual(len(report["hardBlockers"]),6)
            slot=base/"ubi-slot";slot.mkdir()
            payload=b"UBI#"+b"\0"*(131072*3-4)
            (slot/"hgfw-rp2613-experimental-slot.ubi").write_bytes(payload)
            table=[(0,"rootfs_ubifs",160),(1,"METADATA",1),(2,"METADATACOPY",1),(3,"app_ubifs",240),(10,"filestruct_full.bin",31)]
            (slot/"slot-manifest.json").write_text(json.dumps({
                "flashable":False,"installationApproved":False,"boardBootVerified":False,
                "recoveryVerified":False,"hgfwAutostartConfigured":False,
                "image":{"bytes":len(payload),"sha256":hashlib.sha256(payload).hexdigest()},
                "volumes":[{"id":vid,"name":name,"reservedLEBs":reserved} for vid,name,reserved in table],
            }))
            bundled=gate.assess(init,root,app,slot)
            self.assertTrue(bundled["passedLocalArtifactIntegrity"])
            self.assertEqual(len(bundled["artifactIntegrity"]),4)
            self.assertFalse(bundled["installationPermitted"])
            (slot/"hgfw-rp2613-experimental-slot.ubi").write_bytes(payload+b"tampered")
            with self.assertRaises(ValueError):gate.assess(init,root,app,slot)
            (app/"hgfw-rp2613-app-compat-private.tar.gz").write_bytes(b"tampered")
            with self.assertRaises(ValueError):
                gate.assess(init,root,app)

if __name__=="__main__":unittest.main()

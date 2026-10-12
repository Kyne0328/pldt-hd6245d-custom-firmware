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
            (app/"hgfw-rp2613-app-compat-private.tar.gz").write_bytes(b"tampered")
            with self.assertRaises(ValueError):
                gate.assess(init,root,app)

if __name__=="__main__":unittest.main()

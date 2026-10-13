"""RP2613 private app UBIFS packaging safety tests (no real NAND writes)."""
import pathlib
import struct
import sys
import tempfile
import unittest

sys.path.insert(0,str(pathlib.Path(__file__).resolve().parents[1]))
from tools import build_app_ubifs as maker

class AppUBITests(unittest.TestCase):
 def fixture(self,root):
  app=root/"source"
  (app/"fhshell").mkdir(parents=True)
  (app/"bin").mkdir()
  (app/"fhconf").mkdir()
  (app/"fhshell/udevinit.sh").write_text("#!/bin/sh")
  (app/"fhconf/device-config").write_text("PRIVATE")
  b=bytearray(80)
  b[:7]=b"\x7fELF\x01\x01\x01"
  struct.pack_into("<H",b,18,40)
  binary=root/"hgfw"
  binary.write_bytes(b)
  mkfs=root/"mkfs.ubifs"
  mkfs.write_bytes(b"fixture")
  return app,binary,mkfs

 def test_requires_verified_owners_and_isolated_paths(self):
  with tempfile.TemporaryDirectory() as td:
   root=pathlib.Path(td)
   app,binary,mkfs=self.fixture(root)
   stage=root/"stage"
   out=root/"image.ubifs"
   with self.assertRaises(ValueError):
    maker.produce(app,binary,stage,out,mkfs,False)
   with self.assertRaises(ValueError):
    maker.produce(app,binary,stage,stage/"image.ubifs",mkfs,True)

 def test_validates_recovered_app_layout(self):
  with tempfile.TemporaryDirectory() as td:
   root=pathlib.Path(td)
   app,binary,mkfs=self.fixture(root)
   (app/"fhshell/udevinit.sh").unlink()
   with self.assertRaises(ValueError):
    maker.produce(app,binary,root/"out",root/"new.ubifs",mkfs,True)

if __name__=="__main__":unittest.main()

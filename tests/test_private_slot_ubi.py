"""Safety and geometry checks for the experimental RP2613 UBI slot builder."""
from pathlib import Path
import struct
import sys
import tempfile
import unittest

sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from tools import build_private_slot_ubi as ubi

class SlotTests(unittest.TestCase):
 def inputs(self,root):
  result={}
  for _,_,_,_,key in ubi.VOLUMES:
   p=Path(root)/(key+".bin")
   content=bytearray(ubi.LEB if key!="filestruct" else 300)
   if key in ("rootfs","app"):
    struct.pack_into("<I",content,0,ubi.UBIFS_MAGIC)
   p.write_bytes(content)
   result[key]=p
  return result
 def test_slot_geometry_and_vendor_records(self):
  with tempfile.TemporaryDirectory() as td:
   paths=self.inputs(td)
   rows=ubi.check_files(paths)
   self.assertEqual([r["id"] for r in rows],[0,1,2,3,10])
   self.assertEqual([r["reservedLEBs"] for r in rows],[160,1,1,240,31])
   conf=ubi.config_text(rows)
   self.assertIn("vol_id=10",conf)
   self.assertIn("vol_name=filestruct_full.bin",conf)
   self.assertEqual(ubi.LEB,126976)
   self.assertLess(sum(r["reservedLEBs"] for r in rows)+2,ubi.TOTAL_PEB)
 def test_rejects_oversized_rootfs(self):
  with tempfile.TemporaryDirectory() as td:
   p=self.inputs(td)
   p["rootfs"].write_bytes(b"0"*(ubi.LEB*161))
   with self.assertRaises(ValueError):ubi.check_files(p)
 def test_rejects_non_ubifs_and_missing_volumes(self):
  with tempfile.TemporaryDirectory() as td:
   p=self.inputs(td)
   p["app"].write_bytes(b"X"*ubi.LEB)
   with self.assertRaises(ValueError):ubi.check_files(p)
   p["app"].unlink()
   with self.assertRaises(FileNotFoundError):ubi.check_files(p)

if __name__=="__main__":unittest.main()

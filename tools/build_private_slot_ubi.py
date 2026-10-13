#!/usr/bin/env python3
"""Build a private RP2613-geometry UBI slot *candidate*, never a flash image.

Preserves the original kernel/CFE filestruct and metadata volumes, uses the
owner's original rootfs, and inserts an updated private app_ubifs volume
containing HGFW. It is NOT boot-tested, not a vendor firmware update format,
not ECC/OOB complete, and MUST NOT be flashed or sent to an updater.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import struct
import subprocess

PEB = 131072
MIN_IO = 2048
VID_OFFSET = 2048
LEB = PEB - 4096
TOTAL_PEB = 452
IMAGE_SEQ = 1080062015
UBIFS_MAGIC = 0x06101831
VOLUMES = (
 ("rootfs_ubifs",0,"dynamic",160,"rootfs"),
 ("METADATA",1,"dynamic",1,"metadata"),
 ("METADATACOPY",2,"dynamic",1,"metadata_copy"),
 ("app_ubifs",3,"dynamic",240,"app"),
 ("filestruct_full.bin",10,"static",31,"filestruct"),
)

def check_files(paths):
 records=[]
 for name,vid,typ,reserve,key in VOLUMES:
  path=Path(paths[key]).resolve(strict=True)
  if not path.is_file():raise ValueError("missing input UBI volume")
  b=path.read_bytes()
  if not b or len(b)>reserve*LEB:
   raise ValueError(f"{name} exceeds original volume reservation")
  if key in ("rootfs","app"):
   if len(b)%LEB or struct.unpack_from("<I",b)[0]!=UBIFS_MAGIC:
    raise ValueError(f"{name} lacks a valid UBIFS container shape")
  records.append(dict(name=name, id=vid, type=typ, reservedLEBs=reserve,
   bytes=len(b), sha256=hashlib.sha256(b).hexdigest(), path=str(path)))
 if sum(rec["reservedLEBs"] for rec in records)+2>TOTAL_PEB:
  raise ValueError("original UBI slot reservation exceeds NAND")
 return records

def config_text(records):
 lines=[]
 for row in records:
  lines += [
   f"[{row['name']}]", "mode=ubi",f"image={row['path']}",
   f"vol_id={row['id']}",f"vol_size={row['reservedLEBs']*LEB}",
   f"vol_type={row['type']}",f"vol_name={row['name']}", "vol_alignment=1", ""
  ]
 return "\n".join(lines)+"\n"

def build(paths,outdir,ubinize,libpath):
 out=Path(outdir).absolute()
 source=Path(__file__).resolve().parents[1]
 if out==source or source in out.parents:
  raise ValueError("private output cannot be placed inside public source repository")
 if out.exists():raise FileExistsError("refusing overwrite of prior private UBI candidate")
 records=check_files(paths)
 if not Path(ubinize).is_file():raise FileNotFoundError("ubinize binary unavailable")
 out.mkdir(parents=True,mode=0o700)
 out.chmod(0o700)
 config=out/"slot.ini"
 config.write_text(config_text(records),encoding="utf-8")
 config.chmod(0o600)
 image=out/"hgfw-rp2613-experimental-slot.ubi"
 env=os.environ.copy()
 env["LD_LIBRARY_PATH"]=str(libpath)+os.pathsep+env.get("LD_LIBRARY_PATH","")
 args=[str(ubinize),"-o",str(image),"-p",str(PEB),"-m",str(MIN_IO),
       "-s",str(MIN_IO),"-O",str(VID_OFFSET),"-Q",str(IMAGE_SEQ),str(config)]
 saved=os.umask(0o077)
 try:
  subprocess.run(args,check=True,env=env,timeout=180)
 finally:os.umask(saved)
 payload=image.read_bytes()
 if len(payload)<3*PEB or len(payload)>PEB*TOTAL_PEB or len(payload)%PEB:
  raise ValueError("unexpected UBI slot size")
 if payload[:4]!=b"UBI#":
  raise ValueError("UBI EC header missing")
 report={
  "model":"HG6245D RP2613","board":"WKE2.094.277A01",
  "geometry":{"pebBytes":PEB,"lebBytes":LEB,"minIO":MIN_IO,"vidOffset":VID_OFFSET,"originalPEBs":TOTAL_PEB},
  "volumes":[{key:row[key] for key in ("name","id","type","reservedLEBs","bytes","sha256")} for row in records],
  "image":{"bytes":len(payload),"sha256":hashlib.sha256(payload).hexdigest()},
  "includesOriginalVendorKernel":True,"includesOriginalCFERecord":True,
  "hgfwPresentInAppVolume":True,"hgfwAutostartConfigured":False,
  "vendorUpgradeEnvelopeVerified":False,"boardBootVerified":False,
  "gponLanWifiValidated":False,"recoveryVerified":False,
  "flashable":False,"installationApproved":False,
  "containsPrivateProprietaryFirmware":True,
  "note":"Offline engineering image only. Do not flash this UBI file directly or via a firmware updater.",
 }
 manifest=out/"slot-manifest.json"
 manifest.write_text(json.dumps(report,indent=2,sort_keys=True)+"\n",encoding="utf-8")
 manifest.chmod(0o600)
 print("PRIVATE_UBI_SLOT_BUILT",len(payload),"bytes")
 print("PRIVATE_UBI_SLOT_SHA256",report["image"]["sha256"])
 print("FLASH_READY=false")
 return report

def main():
 p=argparse.ArgumentParser(description=__doc__)
 for key in ("rootfs","metadata","metadata-copy","app","filestruct"):
  p.add_argument("--"+key,required=True,type=Path)
 p.add_argument("--output",required=True,type=Path)
 p.add_argument("--ubinize",required=True,type=Path)
 p.add_argument("--lib-dir",required=True,type=Path)
 a=p.parse_args()
 paths={"rootfs":a.rootfs,"metadata":a.metadata,"metadata_copy":a.metadata_copy,
       "app":a.app,"filestruct":a.filestruct}
 build(paths,a.output,a.ubinize,a.lib_dir)

if __name__=="__main__":main()

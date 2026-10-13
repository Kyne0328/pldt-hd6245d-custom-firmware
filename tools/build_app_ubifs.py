#!/usr/bin/env python3
"""Create a PRIVATE experimental RP2613 app_ubifs volume with HGFW included.

Uses the recovered, unchanged stock application tree, including its private
device/application configuration. Output MUST remain outside the public repo.
A successful UBIFS build does not validate update envelopes, startup, GPON,
Wi-Fi, or CFE boot/recovery. NEVER flash this output.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import struct
import subprocess

LEB = 126976
PEB = 131072
MAX_LEBS = 240
UBIFS_MAGIC = 0x06101831


def is_descendant(child, ancestor):
    return child == ancestor or ancestor in child.parents


def produce(stock, hgfw, stage, image, tool, owner_verified):
    if not owner_verified:
        raise ValueError("original app UBIFS owner metadata must be verified before image construction")
    stock = Path(stock).resolve(strict=True)
    hgfw = Path(hgfw).resolve(strict=True)
    stage = Path(stage).absolute()
    image = Path(image).absolute()
    if (not stock.is_dir() or not (stock / "fhshell/udevinit.sh").is_file() or
        not (stock / "bin").is_dir()):
        raise ValueError("original application extraction is incomplete")
    binary = hgfw.read_bytes()
    if len(binary)<60 or binary[:7] != b"\x7fELF\x01\x01\x01" or len(binary) > 12*1024*1024:
        raise ValueError("not an ARM ELF32 HGFW release")
    if struct.unpack_from("<H",binary,18)[0] != 40:
        raise ValueError("HGFW is not ARM")
    repo = Path(__file__).resolve().parents[1]
    if (is_descendant(stage,repo) or is_descendant(image,repo) or
        is_descendant(stage,stock) or is_descendant(stock,stage) or
        is_descendant(image,stage) or is_descendant(stage,image)):
        raise ValueError("private stage/output may not be in source or public repo")
    if stage.exists() or image.exists():
        raise FileExistsError("refusing to overwrite stage or existing image")
    if not Path(tool).is_file():
        raise FileNotFoundError("missing mkfs.ubifs tool")
    shutil.copytree(stock,stage,symlinks=True,copy_function=shutil.copy2)
    stage.chmod(0o700)  # private proprietary and possible credential material
    target=stage/"bin/hgfw"
    if target.exists() or target.is_symlink():
        raise FileExistsError("original app already has bin/hgfw")
    target.write_bytes(binary)
    target.chmod(0o755)
    # Normalize only the newly added file's timestamp. All original app
    # modes and symlinks remain as recovered from the UBIFS image.
    os.utime(target,(0,0))
    args=[str(tool),"-m","2048","-e",str(LEB),"-c",str(MAX_LEBS),
          "-x","lzo","-U","-r",str(stage),"-o",str(image)]
    saved_umask=os.umask(0o077)
    try:
        subprocess.run(args,check=True,timeout=180)
    finally:
        os.umask(saved_umask)
    image.chmod(0o600)
    out=image.read_bytes()
    if len(out)<LEB or len(out)>LEB*MAX_LEBS or len(out)%LEB:
        raise ValueError("UBIFS image exceeds original volume bounds")
    if struct.unpack_from("<I",out,0)[0]!=UBIFS_MAGIC:
        raise ValueError("result lacks UBIFS superblock magic")
    return {
      "targetBoard":"WKE2.094.277A01",
      "sourceVolume":"app_ubifs",
      "volumeId":3,
      "minIO":2048,"logicalEraseBlock":LEB,"reservedLEBs":MAX_LEBS,
      "imageBytes":len(out),"logicalEraseBlocks":len(out)//LEB,
      "sha256":hashlib.sha256(out).hexdigest(),
      "hgfwSHA256":hashlib.sha256(binary).hexdigest(),
      "privateProprietaryFiles":True,"mayContainFactoryConfig":True,
      "startsHGFWAutomatically":False,
      "bootedOnHardware":False,"flashable":False,
      "installationApproved":False,
      "verifiedRecovery":False,
    }


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument("--stock-app",type=Path,required=True)
    p.add_argument("--hgfw",type=Path,required=True)
    p.add_argument("--stage",type=Path,required=True)
    p.add_argument("--image",type=Path,required=True)
    p.add_argument("--mkfs",type=Path,required=True)
    p.add_argument("--owners-verified",action="store_true")
    args=p.parse_args()
    result=produce(args.stock_app,args.hgfw,args.stage,args.image,args.mkfs,args.owners_verified)
    manifest=Path(str(args.image)+".json")
    manifest.write_text(json.dumps(result,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    print("APP_UBIFS_CONSTRUCTED",result["imageBytes"],"bytes")
    print("APP_UBIFS_USED_LEBS",result["logicalEraseBlocks"],"/",MAX_LEBS)
    print("APP_UBIFS_SHA256",result["sha256"])
    print("FLASH_READY=false")


if __name__=="__main__":main()

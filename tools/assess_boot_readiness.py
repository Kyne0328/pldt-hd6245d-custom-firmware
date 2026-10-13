#!/usr/bin/env python3
"""Fail-closed offline installation-readiness assessment for HGFW RP2613.

Offline component archives never imply flash compatibility. Only source and
manifest checks occur; this tool cannot authorize flashing or modify hardware.
"""
import argparse
import hashlib
import json
from pathlib import Path

HARD_BLOCKERS = (
    ("bootloader", "No observed RP2613 loader acceptance of the HGFW kernel/initramfs"),
    ("recovery", "No demonstrated non-UART RP2613 recovery after a bad NAND image"),
    ("envelope", "Vendor update signatures/checksums and active-slot policy not verified"),
    ("kernel", "No independently booted HGFW kernel with board ABI and NAND support"),
    ("services", "WAN/LAN switching, Broadcom Wi-Fi/runner and network service cutover unverified"),
    ("provisioning", "Safe preservation/migration of proprietary data and factory calibration not verified"),
)

def check_identity(folder, name, metadata):
    manifest = json.loads((Path(folder) / metadata).read_text(encoding="utf-8"))
    artifact = manifest.get("archive", {})
    path = Path(folder) / name
    contents = path.read_bytes()
    if (manifest.get("bootable") or manifest.get("flashable") or
        artifact.get("file") != name or artifact.get("bytes") != len(contents) or
        artifact.get("sha256") != hashlib.sha256(contents).hexdigest()):
        raise ValueError("incomplete, tampered or mischaracterized offline artifact")
    return {"bytes": len(contents), "sha256": artifact["sha256"]}

def check_ubi_slot(folder):
    folder=Path(folder)
    manifest=json.loads((folder/"slot-manifest.json").read_text())
    blob=(folder/"hgfw-rp2613-experimental-slot.ubi").read_bytes()
    size=len(blob)
    if (manifest.get("flashable") or manifest.get("installationApproved") or
        manifest.get("boardBootVerified") or manifest.get("recoveryVerified") or
        manifest.get("hgfwAutostartConfigured") or blob[:4]!=b"UBI#" or
        size<3*131072 or size>452*131072 or size%131072 or
        manifest.get("image",{}).get("bytes")!=size or
        manifest.get("image",{}).get("sha256")!=hashlib.sha256(blob).hexdigest()):
        raise ValueError("UBI image identity or safety state invalid")
    ids=[(v["id"],v["name"],v["reservedLEBs"]) for v in manifest["volumes"]]
    if ids != [(0,"rootfs_ubifs",160),(1,"METADATA",1),(2,"METADATACOPY",1),
               (3,"app_ubifs",240),(10,"filestruct_full.bin",31)]:
        raise ValueError("unexpected RP2613 UBI volume table")
    return {"bytes":size,"sha256":manifest["image"]["sha256"],"bootVerified":False}

def assess(initramfs, rootfs, app, slot=None):
    findings = {
       "target":"FiberHome HG6245D RP2613 WKE2.094.277A01",
       "mode":"offline-validation-only",
       "flashable":False,
       "installationPermitted":False,
       "bootVerified":False,
       "recoveryVerified":False,
       "passedLocalArtifactIntegrity":False,
       "hardBlockers":[{"id":name,"reason":reason} for name,reason in HARD_BLOCKERS],
    }
    verified = {}
    verified["initramfs"] = check_identity(
       initramfs, "hgfw-experimental-initramfs-armv7.cpio.gz",
       "hgfw-experimental-initramfs-manifest.json")
    verified["rootfs"] = check_identity(
       rootfs, "hgfw-rp2613-hybrid-rootfs-private.tar.gz",
       "hgfw-rp2613-hybrid-rootfs-private.json")
    verified["app"] = check_identity(
       app, "hgfw-rp2613-app-compat-private.tar.gz",
       "hgfw-rp2613-app-compat-private.json")
    if slot is not None:
        verified["ubiSlot"]=check_ubi_slot(slot)
    findings["artifactIntegrity"] = verified
    findings["passedLocalArtifactIntegrity"] = True
    return findings

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--initramfs",required=True,type=Path)
    parser.add_argument("--rootfs",required=True,type=Path)
    parser.add_argument("--app",required=True,type=Path)
    parser.add_argument("--output",type=Path,required=True)
    parser.add_argument("--ubi-slot",type=Path)
    args=parser.parse_args()
    report=assess(args.initramfs,args.rootfs,args.app,args.ubi_slot)
    args.output.parent.mkdir(parents=True,exist_ok=True)
    args.output.write_text(json.dumps(report,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    print("OFFLINE_ARCHIVES_VERIFIED",len(report["artifactIntegrity"]))
    print("INSTALL_READY=false")
    print("HARD_BLOCKERS",len(report["hardBlockers"]))

if __name__=="__main__": main()

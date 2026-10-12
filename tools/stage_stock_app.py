#!/usr/bin/env python3
"""Stage the RP2613 original application UBIFS payload for offline integration.

Private stock binaries remain private; mounted /fhconf data and customer
provisioning are excluded. No device process starts, no changes to stock
boot/active slot markers, and no flashable image is constructed.
"""
import argparse
import json
from pathlib import Path
import shutil
import sys

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
from stage_hybrid_rootfs import inventory

BLOCKED = {"fhconf", "tr069", "credentials", "secrets"}

def stage(source, target, owners_verified=False):
    if not owners_verified:
        raise ValueError("refusing inferred owner metadata; verify original app_ubifs inodes first")
    source = Path(source).resolve(strict=True)
    target = Path(target).absolute()
    if not source.is_dir() or not (source / "fhshell/udevinit.sh").is_file():
        raise ValueError("not an expected RP2613 app UBIFS extraction")
    if target.exists() or target.is_symlink() or source in target.parents or target in source.parents:
        raise FileExistsError("destination overlaps source or exists")
    if (source / "fhconf").exists():
        # Observed app volume includes a top-level fhconf. It is *never* staged.
        pass
    def ignore(directory, names):
        if Path(directory) == source:
            return BLOCKED.intersection(names)
        return set()
    dest = target / "rootfs"
    dest.parent.mkdir(parents=True, exist_ok=False)
    shutil.copytree(source, dest, symlinks=True, ignore=ignore)
    files = inventory(dest)
    manifest = {
       "schema": 1, "kind": "private-rp2613-app-ubifs-compat",
       "bootable": False, "flashable": False, "factoryProvisioningIncluded": False,
       "automaticallyStartsHGFW": False, "modeBitsRestoredFromUBIFS": True,
       "originalAppOwnersAllRootVerified": True,
       "privateDoNotPublish": True, "files": files,
    }
    (target / "manifest.json").write_text(json.dumps(manifest,indent=2,sort_keys=True)+"\n")
    print("STAGED_APP_FILES",sum(row["kind"]=="file" for row in files))
    print("STAGED_APP_SYMLINKS",sum(row["kind"]=="symlink" for row in files))
    return manifest

def verify(target):
    target = Path(target)
    manifest = json.loads((target / "manifest.json").read_text())
    if manifest.get("bootable") or manifest.get("flashable") or manifest.get("factoryProvisioningIncluded"):
        raise ValueError("invalid safety markers")
    if inventory(target / "rootfs") != manifest["files"]:
        raise ValueError("app staging differs from manifest")
    print("VERIFIED_APP_STAGING",len(manifest["files"]))
    return True

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action",choices=("stage","verify"))
    parser.add_argument("--source",type=Path)
    parser.add_argument("--output",type=Path,required=True)
    parser.add_argument("--all-root-owners-verified",action="store_true")
    args = parser.parse_args()
    if args.action=="stage":
        if args.source is None: parser.error("stage requires --source")
        stage(args.source,args.output,args.all_root_owners_verified)
    else:
        verify(args.output)

if __name__=="__main__": main()

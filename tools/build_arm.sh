#!/usr/bin/env bash
# Build ONLY on the developer PC, not on the router.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$ROOT/build"
command -v clang >/dev/null || { echo 'clang unavailable' >&2; exit 1; }
command -v ld.lld >/dev/null || { echo 'lld unavailable' >&2; exit 1; }
clang --target=armv7a-linux-gnueabi -march=armv7-a -marm \
    -nostdlib -static -fuse-ld=lld -Wl,--build-id=none -Wl,-e,_start \
    -Wl,-z,noexecstack -o "$ROOT/build/hgfw-canary" "$ROOT/src/arm/hello.S"
python3 - "$ROOT/build/hgfw-canary" <<'PY'
import sys, struct, pathlib
p = pathlib.Path(sys.argv[1])
b = p.read_bytes()
assert b[:4] == b'\x7fELF' and b[4] == 1 and b[5] == 1, 'not ELF32 little endian'
assert struct.unpack_from('<H', b, 18)[0] == 40, 'not ARM machine'
print('ARM Linux ELF generated:', p, 'bytes=', len(b))
PY

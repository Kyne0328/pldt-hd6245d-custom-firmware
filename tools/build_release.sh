#!/bin/sh
# Host-only HGFW release verification. Never contacts a router or modifies NAND.
set -eu
cd "$(dirname "$0")/.."
GO="${GO:-go}"
PYTHON="${PYTHON:-python3}"
mkdir -p build

echo "[1/6] Go and device regression tests"
"$GO" test ./... -count=1
"$GO" vet ./...

echo "[2/6] Independent Python tests and sender syntax"
"$PYTHON" -m unittest discover -s tests -v
"$PYTHON" -m py_compile tools/hgfw_send.py

echo "[3/6] Browser code syntax"
if command -v node >/dev/null 2>&1; then
  node --check internal/admin/static/app.js
else
  echo "NODE_UNAVAILABLE: skipped JS syntax check"
fi

echo "[4/6] Host and ARMv7 softfloat binaries"
CGO_ENABLED=0 "$GO" build -trimpath -ldflags="-s -w" -o build/hgfw-host-next ./cmd/hgfw
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7,softfloat "$GO" build -trimpath -ldflags="-s -w" -o build/hgfw-armv7-next ./cmd/hgfw
"$GO" version -m build/hgfw-armv7-next | grep -F "GOARM=7,softfloat"

echo "[5/6] Host integration (DNS + supervised loopback + authentication)"
./build/hgfw-host-next field-check

echo "[6/6] ARM emulation if QEMU is configured"
if [ -n "${QEMU_ARM:-}" ] && [ -x "${QEMU_ARM}" ]; then
  "${QEMU_ARM}" build/hgfw-armv7-next dns-selftest
  "${QEMU_ARM}" build/hgfw-armv7-next smoke
else
  echo "QEMU_ARM_UNSET: QEMU checks skipped, not marked passed"
fi

echo "HGFW_ARM_RELEASE_BYTES=$(wc -c < build/hgfw-armv7-next)"
printf "HGFW_ARM_RELEASE_SHA256="
sha256sum build/hgfw-armv7-next | cut -d ' ' -f1
echo "HOST_CHECKS_COMPLETED"

# HGFW host-only cross build. Never interacts with router/NAND.
PYTHON ?= python3
GO ?= go
.PHONY: all arm go-bin host overlay compat verify test clean

all: arm go-bin overlay verify test

arm:
	bash tools/build_arm.sh

go-bin:
	mkdir -p build
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7,softfloat $(GO) build -trimpath -ldflags="-s -w" -o build/hgfw-armv7 ./cmd/hgfw
	$(GO) version -m build/hgfw-armv7 | grep -F 'GOARM=7,softfloat'

host:
	mkdir -p build
	CGO_ENABLED=0 $(GO) build -trimpath -o build/hgfw-host ./cmd/hgfw

overlay: arm go-bin
	$(PYTHON) fwkit.py build

verify:
	$(PYTHON) fwkit.py verify

test:
	$(GO) test ./...
	$(PYTHON) -m unittest discover -s tests -v

# Optional PRIVATE stock BusyBox may be used for compatibility research.
compat: overlay
	$(PYTHON) compat_rootfs.py build
	$(PYTHON) compat_rootfs.py verify

clean:
	$(PYTHON) -c "import shutil; shutil.rmtree('build', ignore_errors=True); shutil.rmtree('dist', ignore_errors=True)"

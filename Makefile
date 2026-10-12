# HGFW offline-only build. Neither the router nor NAND is accessed.
PYTHON ?= python3
.PHONY: all arm overlay compat verify verify-compat test clean

all: overlay verify test

arm:
	bash tools/build_arm.sh

overlay: arm
	$(PYTHON) fwkit.py build

verify:
	$(PYTHON) fwkit.py verify

test:
	$(PYTHON) -m unittest discover -s tests -v

# Optional: requires HGFW_STOCK_BUSYBOX pointing to an authorized local copy.
compat: overlay
	$(PYTHON) compat_rootfs.py build
	$(PYTHON) compat_rootfs.py verify

verify-compat:
	$(PYTHON) compat_rootfs.py verify

clean:
	$(PYTHON) -c "import shutil; shutil.rmtree('build', ignore_errors=True); shutil.rmtree('dist', ignore_errors=True)"

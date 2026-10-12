package storage

import (
	"errors"
	"fmt"
	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/device"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var partRE = regexp.MustCompile(`^/dev/sd[a-z][1-9][0-9]*$`)

// MountReadOnly is an explicit CLI-only action, never run at service start or over HTTP.
func MountReadOnly(partition, root string) (string, error) {
	if !partRE.MatchString(partition) {
		return "", errors.New("invalid USB partition path")
	}
	if e := device.ValidateUSBPartition(partition); e != nil {
		return "", e
	}
	for _, disk := range device.USBDisks("/sys/class/block", "/proc/mounts") {
		if disk.Device == partition && disk.Mountpoint != "" {
			return "", errors.New("USB partition already mounted; refusing duplicate mount")
		}
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) == "/" {
		return "", errors.New("unsafe mount root")
	}
	real, e := filepath.EvalSymlinks(root)
	if e != nil {
		return "", e
	}
	if real != filepath.Clean(root) {
		return "", errors.New("symlink mount root forbidden")
	}
	t := filepath.Join(root, filepath.Base(partition))
	if e = os.Mkdir(t, 0700); e != nil && !os.IsExist(e) {
		return "", e
	}
	real, e = filepath.EvalSymlinks(t)
	if e != nil || real != t {
		return "", errors.New("unsafe mount target")
	}
	cmd := exec.Command("/bin/mount", "-o", "ro,nosuid,nodev,noexec", partition, t)
	if b, e := cmd.CombinedOutput(); e != nil {
		return "", fmt.Errorf("mount failed: %s: %w", strings.TrimSpace(string(b)), e)
	}
	return t, nil
}
func Unmount(target, root string) error {
	realRoot, e := filepath.EvalSymlinks(root)
	if e != nil {
		return e
	}
	realTarget, e := filepath.EvalSymlinks(target)
	if e != nil {
		return e
	}
	if filepath.Dir(realTarget) != realRoot || !partRE.MatchString("/dev/"+filepath.Base(realTarget)) {
		return errors.New("unrecognized mount target")
	}
	cmd := exec.Command("/bin/umount", realTarget)
	if b, e := cmd.CombinedOutput(); e != nil {
		return fmt.Errorf("unmount failed: %s: %w", strings.TrimSpace(string(b)), e)
	}
	return nil
}

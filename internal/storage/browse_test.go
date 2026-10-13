package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/device"
)

func TestUSBBrowseConfinedAndReadOnly(t *testing.T) {
	root := t.TempDir()
	mp := filepath.Join(root, "sda1")
	if err := os.MkdirAll(filepath.Join(mp, "Photos"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mp, "hello.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mp, ".credentials"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(mp, "escape")); err != nil {
		t.Fatal(err)
	}
	d := []device.Disk{{Name: "sda1", Device: "/dev/sda1", Partition: true, Mountpoint: mp}}
	list, err := Browse(root, "sda1", "", d)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Entries) != 2 {
		t.Fatalf("unexpected list %+v", list.Entries)
	}
	f, n, err := OpenFile(root, "sda1", "hello.txt", d)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if n != 5 {
		t.Fatal("bad length")
	}
	for _, v := range []string{"../etc/passwd", "/etc/passwd", "escape/foo", "foo/../../etc"} {
		if _, err := Browse(root, "sda1", v, d); err == nil {
			t.Fatalf("traversal allowed: %s", v)
		}
		if f, _, err := OpenFile(root, "sda1", v, d); err == nil {
			_ = f.Close()
			t.Fatalf("file traversal allowed: %s", v)
		}
	}
	if _, err := Browse(root, "sda1", "", nil); err == nil {
		t.Fatal("invented mount")
	}
	d[0].Mountpoint = "/tmp/nonmatching"
	if _, err := Browse(root, "sda1", "", d); err == nil {
		t.Fatal("accepted unverified mount")
	}
}
func TestUSBBrowseRejectsMountSymlink(t *testing.T) {
	root := t.TempDir()
	actual := t.TempDir()
	link := filepath.Join(root, "sdb1")
	if err := os.Symlink(actual, link); err != nil {
		t.Fatal(err)
	}
	d := []device.Disk{{Name: "sdb1", Device: "/dev/sdb1", Partition: true, Mountpoint: link}}
	if _, err := Browse(root, "sdb1", "", d); err == nil {
		t.Fatal("mount symlink accepted")
	}
}

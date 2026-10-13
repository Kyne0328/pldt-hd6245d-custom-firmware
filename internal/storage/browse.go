package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/device"
)

type FileEntry struct {
	Name      string `json:"name"`
	Directory bool   `json:"directory"`
	Bytes     int64  `json:"bytes"`
}
type Directory struct {
	Partition string      `json:"partition"`
	Path      string      `json:"path"`
	Entries   []FileEntry `json:"entries"`
}

func checkedRoot(root, partition, relative string, disks []device.Disk) (string, string, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) == "/" {
		return "", "", errors.New("invalid USB root")
	}
	if strings.ContainsAny(partition, "/\\") || !partRE.MatchString("/dev/"+partition) {
		return "", "", errors.New("invalid partition")
	}
	valid := false
	mount := filepath.Join(filepath.Clean(root), partition)
	for _, d := range disks {
		if d.Name == partition && d.Device == "/dev/"+partition && d.Partition && filepath.Clean(d.Mountpoint) == mount {
			valid = true
			break
		}
	}
	if !valid {
		return "", "", errors.New("partition not mounted in HGFW USB directory")
	}
	if strings.Contains(relative, "\\") || strings.HasPrefix(relative, "/") {
		return "", "", errors.New("invalid relative file path")
	}
	if relative == "" {
		relative = "."
	}
	if !filepath.IsLocal(relative) {
		return "", "", errors.New("file must remain inside mounted USB partition")
	}
	real, err := filepath.EvalSymlinks(mount)
	if err != nil || real != mount {
		return "", "", errors.New("USB mount path changed")
	}
	return mount, relative, nil
}

// Browse presents only an explicitly mounted USB partition. A descriptor-bound
// os.Root prevents directory traversal and symlink escapes. No files are changed.
func Browse(root, partition, relative string, disks []device.Disk) (Directory, error) {
	mount, rel, err := checkedRoot(root, partition, relative, disks)
	if err != nil {
		return Directory{}, err
	}
	sandbox, err := os.OpenRoot(mount)
	if err != nil {
		return Directory{}, err
	}
	defer sandbox.Close()
	dir, err := sandbox.Open(rel)
	if err != nil {
		return Directory{}, err
	}
	defer dir.Close()
	stat, err := dir.Stat()
	if err != nil || !stat.IsDir() {
		return Directory{}, errors.New("not a directory")
	}
	entries, readErr := dir.ReadDir(129)
	if readErr != nil && readErr != io.EOF {
		return Directory{}, readErr
	}
	if len(entries) > 128 {
		return Directory{}, errors.New("directory exceeds 128 entries")
	}
	result := Directory{Partition: partition, Path: strings.TrimPrefix(rel, "./"), Entries: []FileEntry{}}
	if result.Path == "." {
		result.Path = ""
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			continue
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		} // avoid hidden config files
		result.Entries = append(result.Entries, FileEntry{Name: entry.Name(), Directory: info.IsDir(), Bytes: info.Size()})
	}
	sort.Slice(result.Entries, func(i, j int) bool {
		if result.Entries[i].Directory != result.Entries[j].Directory {
			return result.Entries[i].Directory
		}
		return strings.ToLower(result.Entries[i].Name) < strings.ToLower(result.Entries[j].Name)
	})
	return result, nil
}

// OpenFile serves a bounded, regular file from a confirmed USB mount.
// Caller is responsible for closing the returned file.
func OpenFile(root, partition, relative string, disks []device.Disk) (*os.File, int64, error) {
	mount, rel, err := checkedRoot(root, partition, relative, disks)
	if err != nil {
		return nil, 0, err
	}
	if rel == "." {
		return nil, 0, errors.New("a filename is required")
	}
	sandbox, err := os.OpenRoot(mount)
	if err != nil {
		return nil, 0, err
	}
	defer sandbox.Close()
	f, err := sandbox.Open(rel)
	if err != nil {
		return nil, 0, err
	}
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() > 32*1024*1024 || stat.Size() < 0 {
		_ = f.Close()
		return nil, 0, fmt.Errorf("not a supported file (max 32 MiB)")
	}
	return f, stat.Size(), nil
}

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Disk is a candidate whole-disk block device.
type Disk struct {
	Path  string
	Model string
	Size  int64 // bytes
}

// minDiskBytes: devices smaller than this (e.g. tiny blank disks) are ignored.
const minDiskBytes = 256 << 20

// listDisks enumerates and filters whole block devices under /sys/block.
func listDisks() ([]Disk, error) {
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil, err
	}
	mounted := mountedDevices()
	var out []Disk
	for _, e := range entries {
		name := e.Name()
		if skipDevice(name) {
			continue
		}
		sectors, err := readInt(filepath.Join("/sys/block", name, "size"))
		if err != nil || sectors < minDiskBytes/512 {
			continue
		}
		if diskHasMountedPartition(name, mounted) {
			continue // never touch disks that have mounted partitions (protects the host system)
		}
		model := strings.TrimSpace(readString(filepath.Join("/sys/block", name, "device", "model")))
		dev := "/dev/" + name
		if _, err := os.Stat(dev); err != nil {
			continue
		}
		out = append(out, Disk{Path: dev, Model: model, Size: sectors * 512})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// skipDevice excludes device types that are never install targets.
func skipDevice(name string) bool {
	for _, p := range []string{"ram", "loop", "sr", "fd", "zram", "dm-", "md", "mmcblk"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func mountedDevices() map[string]bool {
	m := map[string]bool{}
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return m
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		flds := strings.Fields(sc.Text())
		if len(flds) > 0 {
			m[flds[0]] = true
		}
	}
	return m
}

// diskHasMountedPartition reports whether any partition of the disk is mounted.
func diskHasMountedPartition(name string, mounted map[string]bool) bool {
	ents, err := os.ReadDir("/sys/block/" + name)
	if err != nil {
		return false
	}
	for _, e := range ents {
		// partition subdirs look like <disk>/<disk><part>
		if strings.HasPrefix(e.Name(), name) && mounted["/dev/"+e.Name()] {
			return true
		}
	}
	return false
}

func readInt(path string) (int64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
}

func readString(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func humanSize(b int64) string {
	const G = 1 << 30
	const M = 1 << 20
	switch {
	case b >= G:
		return fmt.Sprintf("%.1f GiB", float64(b)/float64(G))
	default:
		return fmt.Sprintf("%.1f MiB", float64(b)/float64(M))
	}
}

// parseCmdline parses kernel cmdline options prefixed with "aredn.".
func parseCmdline(s string) map[string]string {
	m := map[string]string{}
	for tok := range strings.FieldsSeq(s) {
		if !strings.HasPrefix(tok, "aredn.") {
			continue
		}
		kv := strings.SplitN(tok, "=", 2)
		k := strings.TrimPrefix(kv[0], "aredn.")
		if len(kv) == 2 {
			m[k] = kv[1]
		} else {
			m[k] = "1"
		}
	}
	return m
}

// findImages locates the BIOS and EFI firmware images on the ISO / boot media.
func findImages() (bios, efi string, err error) {
	roots := []string{"/aredn", "/media", "/run/media", "/mnt"}
	var foundB, foundE string
	for _, r := range roots {
		filepath.Walk(r, func(p string, info os.FileInfo, werr error) error {
			if werr != nil {
				return nil
			}
			if info.IsDir() {
				// only scan a few top-level dirs, avoid walking into the system tree
				switch filepath.Base(p) {
				case "proc", "sys", "dev", "var", "usr", "bin", "sbin", "lib", "tmp", "root", "home":
					return filepath.SkipDir
				}
				return nil
			}
			switch {
			case strings.HasSuffix(info.Name(), "combined-efi.img.gz") && foundE == "":
				foundE = p
			case strings.HasSuffix(info.Name(), "-efi.img.gz") && foundE == "":
				// also accept locally-staged names (aredn-efi.img.gz) in addition to official names (*-combined-efi.img.gz)
				foundE = p
			case strings.HasSuffix(info.Name(), "combined.img.gz") && foundB == "":
				foundB = p
			}
			return nil
		})
	}
	if foundB == "" || foundE == "" {
		return "", "", fmt.Errorf("no combined / combined-efi image found on boot media")
	}
	return foundB, foundE, nil
}

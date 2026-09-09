package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// writeImage streams the gzip-compressed firmware into the target block device (dd semantics).
func writeImage(gzPath, devPath string) error {
	f, err := os.Open(gzPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	dev, err := os.OpenFile(devPath, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer dev.Close()

	buf := make([]byte, 1<<20)
	var total int64
	var last int64
	for {
		n, rerr := gz.Read(buf)
		if n > 0 {
			if _, werr := dev.Write(buf[:n]); werr != nil {
				return werr
			}
			total += int64(n)
			if total-last >= 64<<20 {
				fmt.Printf("\r  wrote %s        ", humanSize(total))
				last = total
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if err := dev.Sync(); err != nil {
		return err
	}
	fmt.Printf("\r  wrote %s (uncompressed)  \n", humanSize(total))
	_ = exec.Command("sync").Run()
	return nil
}

// growRootfs offline-expands the rootfs partition to fill the disk (best-effort; validate in qemu before production).
// MBR: rewrite partition 2's last sector. GPT: rewrite partition 2's last sector and **keep the partition GUID**
// (grub.cfg references it via root=PARTUUID=…; changing it breaks boot).
func growRootfs(dev string) error {
	gpt, err := isGPT(dev)
	if err != nil {
		return err
	}
	last := diskSectors(dev) - 1 // last usable sector

	if gpt {
		// 1) move the backup GPT to the new end of the disk
		if out, err := exec.Command("sgdisk", "-e", dev).CombinedOutput(); err != nil {
			return fmt.Errorf("sgdisk -e: %w: %s", err, out)
		}
		// 2) read partition 2's start sector and unique GUID (must be preserved)
		info, err := exec.Command("sgdisk", "-i", "2", dev).Output()
		if err != nil {
			return fmt.Errorf("sgdisk -i 2: %w", err)
		}
		start, guid := gptP2Info(string(info))
		// 3) delete and recreate partition 2, extending it to the end of the disk, keeping start/GUID
		cmd := exec.Command("sgdisk", "-d", "2",
			"-n", fmt.Sprintf("2:%d:0", start),
			"-t", "2:8300",
			"-c", "2:rootfs",
			"-u", "2:"+guid, dev)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("sgdisk recreate partition 2: %w: %s", err, out)
		}
	} else {
		// MBR: dump the current partition table, change only partition 2's size, then write it back
		cur, err := exec.Command("sfdisk", "-d", dev).Output()
		if err != nil {
			return fmt.Errorf("sfdisk -d: %w", err)
		}
		lines := strings.Split(string(cur), "\n")
		target := dev + "2"
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), target+":") {
				lines[i] = sfdiskRewriteSize(l, last)
			}
		}
		cmd := exec.Command("sfdisk", dev)
		cmd.Stdin = strings.NewReader(strings.Join(lines, "\n"))
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("sfdisk write-back: %w: %s", err, out)
		}
	}

	_ = exec.Command("blockdev", "--rereadpt", dev).Run()
	_ = exec.Command("partx", "-u", dev).Run()
	p2 := dev + "2"
	if out, err := exec.Command("e2fsck", "-f", "-y", p2).CombinedOutput(); err != nil {
		return fmt.Errorf("e2fsck: %w: %s", err, out)
	}
	if out, err := exec.Command("resize2fs", p2).CombinedOutput(); err != nil {
		return fmt.Errorf("resize2fs: %w: %s", err, out)
	}
	fmt.Println("  Done: rootfs now fills the whole disk.")
	return nil
}

func isGPT(dev string) (bool, error) {
	f, err := os.Open(dev)
	if err != nil {
		return false, err
	}
	defer f.Close()
	hdr := make([]byte, 520)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return false, err
	}
	return string(hdr[512:516]) == "EFI PART", nil
}

func diskSectors(dev string) int64 {
	v, err := readInt(filepath.Join("/sys/block", filepath.Base(dev), "size"))
	if err != nil {
		return 0
	}
	return v
}

// gptP2Info parses the start sector and unique partition GUID from `sgdisk -i 2` output.
func gptP2Info(out string) (int64, string) {
	var start int64
	var guid string
	for l := range strings.SplitSeq(out, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "First sector:") {
			f := strings.Fields(l)
			if len(f) >= 3 {
				start, _ = strconv.ParseInt(f[2], 10, 64)
			}
		}
		if strings.HasPrefix(l, "Partition unique GUID:") {
			f := strings.Fields(l)
			if len(f) >= 4 {
				guid = f[3]
			}
		}
	}
	return start, guid
}

// sfdiskRewriteSize rewrites a line like
//
//	/dev/vda2 : start= 33792, size= 212992, type=83
//
// so that its size becomes (last-start+1).
func sfdiskRewriteSize(line string, last int64) string {
	head := strings.SplitN(line, "start=", 2)
	if len(head) != 2 {
		return line
	}
	start := int64(0)
	sf := strings.Fields(strings.SplitN(head[1], ",", 2)[0])
	if len(sf) > 0 {
		start, _ = strconv.ParseInt(sf[0], 10, 64)
	}
	newSize := last - start + 1
	rest := strings.SplitN(head[1], "size=", 2)
	if len(rest) != 2 {
		return line
	}
	after := strings.SplitN(rest[1], ",", 2)
	if len(after) != 2 {
		return line
	}
	return fmt.Sprintf("%sstart=%d, size=%d,%s", head[0], start, newSize, after[1])
}

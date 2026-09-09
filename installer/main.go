// aredn-installer — interactive AREDN x86_64 whole-disk installer.
// Runs inside the Alpine live ISO and is responsible for:
//  1. detecting the boot mode (BIOS / UEFI) and picking the combined or combined-efi firmware image;
//  2. enumerating candidate target disks and offering an arrow-key selection menu;
//  3. after double confirmation (typing the disk path + YES), streaming the firmware gzip into the block device (dd);
//  4. optionally growing the rootfs partition offline to fill the disk;
//  5. rebooting automatically when done.
//
// Build (static, no CGO):
//
//	CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o aredn-installer .
//
// Kernel cmdline options (all prefixed with aredn.):
//
//	aredn.auto=1      headless mode: auto-select the first candidate disk
//	aredn.auto=yes    headless mode + skip the double confirmation (dangerous; only for consoles-less panels)
//	aredn.image=efi   force the EFI image; =bios force the BIOS image; =<path> use a custom image
//	aredn.grow=1      expand rootfs to fill the disk after writing
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	opts := parseCmdline(readString("/proc/cmdline"))

	// 1) locate the firmware images (ISO root /aredn/ or any mountable media)
	biosImg, efiImg, err := findImages()
	if err != nil {
		fatal("Cannot find AREDN firmware images:", err)
	}

	// 2) decide which image to use
	img := pickImage(biosImg, efiImg, opts)
	fmt.Printf("Firmware image: %s\n", filepath.Base(img))

	// 3) enumerate candidate disks
	disks, err := listDisks()
	if err != nil {
		fatal("Failed to enumerate disks:", err)
	}
	if len(disks) == 0 {
		fatal("No writable target disk found (>= 256 MiB, no mounted partitions).", nil)
	}

	// 4) select the target disk
	var target *Disk
	auto := opts["auto"] != ""
	if auto {
		target = &disks[0]
		fmt.Printf("Auto mode: selected %s\n", target.Path)
		if opts["auto"] == "yes" {
			fmt.Println("(aredn.auto=yes: skipping confirmation)")
			if err := doInstall(target.Path, img, opts); err != nil {
				fatal("Install failed:", err)
			}
			finish()
			return
		}
	} else {
		target, err = chooseDisk(disks)
		if err != nil {
			fatal("Cancelled:", err)
		}
	}

	// 5) double confirmation: type the disk path + YES
	if !confirmTarget(target.Path) {
		fatal("Cancelled, nothing was written.", nil)
	}

	// 6) write the firmware
	if err := doInstall(target.Path, img, opts); err != nil {
		fatal("Install failed:", err)
	}

	// 7) finish
	finish()
}

// pickImage decides which firmware image to use based on the boot mode and user options.
func pickImage(biosImg, efiImg string, opts map[string]string) string {
	switch opts["image"] {
	case "bios":
		return biosImg
	case "efi":
		return efiImg
	}
	if opts["image"] != "" {
		return opts["image"] // custom path
	}
	if _, err := os.Stat("/sys/firmware/efi"); err == nil {
		return efiImg // UEFI boot
	}
	return biosImg
}

// doInstall performs the write (and the optional rootfs expansion).
func doInstall(dev, img string, opts map[string]string) error {
	fmt.Printf(">>> Writing %s to %s ...\n", filepath.Base(img), dev)
	if err := writeImage(img, dev); err != nil {
		return err
	}
	fmt.Println(">>> Write complete, sync done.")
	if opts["grow"] == "1" {
		fmt.Println(">>> Expanding rootfs to fill the disk ...")
		if err := growRootfs(dev); err != nil {
			return fmt.Errorf("expansion failed (can be ignored, firmware is already installed): %w", err)
		}
	}
	return nil
}

// confirmTarget asks the user to type the target disk path and then YES.
func confirmTarget(path string) bool {
	fmt.Println("\n!!! This will OVERWRITE the entire target disk with AREDN firmware. ALL DATA ON THE DISK WILL BE DESTROYED !!!")
	got, err := readLine(fmt.Sprintf("Type the target disk path to confirm (%s)", path))
	if err != nil || got != path {
		fmt.Println("Confirmation failed: path does not match. Aborted.")
		return false
	}
	got, err = readLine("Type YES to confirm")
	if err != nil || !strings.EqualFold(got, "YES") {
		fmt.Println("Confirmation failed: 'YES' not entered. Aborted.")
		return false
	}
	return true
}

// finish prints the summary and reboots after 15 seconds (any key + Enter cancels and keeps it running).
func finish() {
	fmt.Println("\n==================================================")
	fmt.Println("AREDN firmware has been written.")
	fmt.Println("1) Detach/unmount this ISO in your provider panel;")
	fmt.Println("2) The machine will reboot automatically in 15 seconds")
	fmt.Println("   (press any key + Enter to cancel and stay here);")
	fmt.Println("3) After boot, the node defaults to 192.168.1.1 (localnode.local.mesh).")
	fmt.Println("==================================================")

	cancel := make(chan struct{})
	go func() {
		r := bufio.NewReader(os.Stdin)
		_, _ = r.ReadString('\n')
		close(cancel)
	}()

	select {
	case <-cancel:
		fmt.Println("Staying here. Run 'reboot' or 'exit' anytime.")
	case <-time.After(15 * time.Second):
		fmt.Println("Rebooting ...")
		runReboot()
	}
}

// runReboot tries several reboot methods in order until the system starts rebooting.
func runReboot() {
	cmds := [][]string{
		{"/sbin/reboot"}, // busybox reboot, graceful reboot
		{"reboot"},
		{"/sbin/reboot", "-f"},
		{"reboot", "-f"},
	}
	for _, c := range cmds {
		if err := exec.Command(c[0], c[1:]...).Run(); err == nil {
			select {} // system is rebooting; block until restart
		}
	}
	// last resort: kernel sysrq hard reboot
	_ = os.WriteFile("/proc/sysrq-trigger", []byte("b"), 0644)
	select {}
}

func fatal(prefix string, err error) {
	fmt.Fprintf(os.Stderr, "%s %v\n", prefix, err)
	os.Exit(1)
}

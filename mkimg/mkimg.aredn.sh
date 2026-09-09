#!/bin/sh
# mkimg.aredn.sh — custom Alpine mkimage profile: the AREDN x86_64 installer ISO.
# Placed at $HOME/.mkimage/mkimg.aredn.sh by build.sh, used via `mkimage.sh --profile aredn`.
# Reference: https://wiki.alpinelinux.org/wiki/How_to_make_a_custom_ISO_image_with_mkimage

profile_aredn() {
	profile_standard
	title="AREDN x86_64 Installer"
	desc="Bootable installer: pick a disk, confirm, and dd the AREDN firmware onto it."
	image_name="aredn-${AREDN_VERSION:-dev}-x86_64-installer"
	kernel_cmdline="console=tty0 console=ttyS0,115200"
	initfs_cmdline="modules=loop,squashfs,sd-mod,usb-storage,isofs,sr_mod"
	syslinux_serial="0 115200"
	hostname="aredn-installer"

	# tools needed in the live system: disk selection / partitioning / resizing
	apks="$apks util-linux gptfdisk e2fsprogs gzip"
	apkovl="genapkovl-aredn.sh"
}

# Put the firmware images and the installer binary into the ISO root /aredn/
# (via the section mechanism, stored on disk rather than in boot RAM)
section_aredn() {
	[ -n "$AREDN_EXTRA" ] && [ -d "$AREDN_EXTRA" ] || return 0
	build_section aredn extra
}

build_aredn() {
	mkdir -p "$DESTDIR/aredn"
	cp -a "$AREDN_EXTRA"/* "$DESTDIR/aredn/"
}
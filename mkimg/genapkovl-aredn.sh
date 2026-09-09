#!/bin/sh -e
# genapkovl-aredn.sh — Alpine live overlay layer:
#   1) declares the packages to install in the live system (/etc/apk/world);
#   2) sets hostname / network / motd;
#   3) places the Go installer at /sbin/aredn-installer;
#   4) uses inittab so that every available console (VGA tty1..tty6 + serial ttyS0/ttyS1)
#      auto-enters the installer on boot; Ctrl-C drops to a root shell on that console.
#    To disable auto-start, boot with aredn.autorun=0.
# Reference: aports/scripts/genapkovl-dhcp.sh, aports/main/alpine-baselayout/inittab

HOSTNAME="${1:-aredn-installer}"

cleanup() { rm -rf "$tmp"; }
makefile() {
	OWNER="$1"; PERMS="$2"; FILE="$3"
	cat > "$FILE"
	chown "$OWNER" "$FILE"
	chmod "$PERMS" "$FILE"
}
rc_add() {
	mkdir -p "$tmp"/etc/runlevels/"$2"
	ln -sf /etc/init.d/"$1" "$tmp"/etc/runlevels/"$2"/"$1"
}

tmp="$(mktemp -d)"
trap cleanup EXIT

mkdir -p "$tmp"/etc
makefile root:root 0644 "$tmp"/etc/hostname <<EOF
$HOSTNAME
EOF

mkdir -p "$tmp"/etc/network
makefile root:root 0644 "$tmp"/etc/network/interfaces <<EOF
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet dhcp
EOF

mkdir -p "$tmp"/etc/apk
makefile root:root 0644 "$tmp"/etc/apk/world <<EOF
alpine-base
util-linux
gptfdisk
e2fsprogs
gzip
EOF

# Go installer (from AREDN_EXTRA, exported by build.sh)
if [ -n "$AREDN_EXTRA" ] && [ -f "$AREDN_EXTRA/aredn-installer" ]; then
	mkdir -p "$tmp"/sbin
	cp "$AREDN_EXTRA/aredn-installer" "$tmp"/sbin/aredn-installer
	chmod 0755 "$tmp"/sbin/aredn-installer
fi

# Auto-enter the installer on every console (replaces getty; Ctrl-C drops to a root shell on that console)
mkdir -p "$tmp"/etc
makefile root:root 0644 "$tmp"/etc/inittab <<'EOF'
::sysinit:/sbin/openrc sysinit
::sysinit:/sbin/openrc boot
::wait:/sbin/openrc default

# AREDN Installer: auto-start on every available console (replaces getty)
# Ctrl-C exits the installer and leaves a root shell on that console
tty1::once:/bin/sh -c "/sbin/aredn-installer; /bin/sh"
tty2::once:/bin/sh -c "/sbin/aredn-installer; /bin/sh"
tty3::once:/bin/sh -c "/sbin/aredn-installer; /bin/sh"
tty4::once:/bin/sh -c "/sbin/aredn-installer; /bin/sh"
tty5::once:/bin/sh -c "/sbin/aredn-installer; /bin/sh"
tty6::once:/bin/sh -c "/sbin/aredn-installer; /bin/sh"
ttyS0::once:/bin/sh -c "/sbin/aredn-installer; /bin/sh"
ttyS1::once:/bin/sh -c "/sbin/aredn-installer; /bin/sh"

# 3-finger salute
::ctrlaltdel:/sbin/reboot

::shutdown:/sbin/openrc shutdown
EOF

makefile root:root 0644 "$tmp"/etc/motd <<EOF
====================================================
 AREDN x86_64 Installer
   - The installer starts automatically on boot
     (select a disk, type the disk path, type YES)
   - To exit: press Ctrl-C to get a root shell
   - To disable auto-start: boot with aredn.autorun=0
   - After installation: detach this ISO in your
     provider panel, then power the machine back on
====================================================
EOF

rc_add devfs sysinit
rc_add dmesg sysinit
rc_add mdev sysinit
rc_add hwdrivers sysinit
rc_add modloop sysinit

rc_add hwclock boot
rc_add modules boot
rc_add sysctl boot
rc_add hostname boot
rc_add bootmisc boot
rc_add syslog boot

rc_add mount-ro shutdown
rc_add killprocs shutdown
rc_add savecache shutdown

tarargs="etc"
[ -d "$tmp/sbin" ] && tarargs="$tarargs sbin"
tar -c -C "$tmp" $tarargs | gzip -9n > "$HOSTNAME.apkovl.tar.gz"

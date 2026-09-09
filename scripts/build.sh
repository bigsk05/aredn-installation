#!/bin/sh -e
# build.sh — builds the custom installer ISO inside an Alpine container (container entrypoint).
# Usage: build.sh <release|nightly>
# Prereq: the host mounts the repo at /work; artifacts/<src>/ contains *.img.gz, aredn-installer and VERSION
set -e

SRCDIR="${1:?usage: build.sh <release|nightly>}"
export AREDN_EXTRA="/work/artifacts/$SRCDIR"
export AREDN_VERSION="$(cat "$AREDN_EXTRA/VERSION")"

echo "== installing build tools =="
apk add --no-cache abuild alpine-conf xorriso squashfs-tools grub grub-efi mtools git fakeroot mkinitfs curl >/dev/null
# grub-mkimage needs the target platform modules on the host (/usr/lib/grub/x86_64-efi);
# the grub-efi package provides them - extract manually as a fallback just in case
if [ ! -e /usr/lib/grub/x86_64-efi/moddep.lst ]; then
	apk fetch --stdout grub-efi 2>/dev/null | tar -xz -C / usr/lib/grub 2>/dev/null || true
fi

echo "== preparing signing keys and mkimage profile =="
adduser -D builder 2>/dev/null || true
chmod -R a+rwX /work 2>/dev/null || true

fetch_aports() {
	# Must match the Alpine release being used: master's mkimage.sh needs a newer
	# update-kernel option (--cache-dir) that alpine-conf on v3.20 does not support,
	# so use the 3.20-stable branch.
	# gitlab.alpinelinux.org returns HTTP 418 for automated clones from cloud CI
	# (e.g. GitHub hosted runners), so download the branch tarball from the GitHub
	# mirror instead; git clone from the mirror is the fallback.
	local url
	for url in \
		"https://codeload.github.com/alpinelinux/aports/tar.gz/refs/heads/3.20-stable" \
		"https://gitlab.alpinelinux.org/alpine/aports/-/archive/3.20-stable/aports-3.20-stable.tar.gz" \
		; do
		if curl -fsSL --retry 3 -o /tmp/aports.tgz "$url" && tar -tzf /tmp/aports.tgz scripts/mkimage.sh >/dev/null 2>&1; then
			mkdir -p /aports
			tar -xzf /tmp/aports.tgz -C /aports --strip-components=1
			rm -f /tmp/aports.tgz
			return 0
		fi
	done
	# last resort: git clone from the GitHub mirror
	git clone --depth 1 --branch 3.20-stable https://github.com/alpinelinux/aports.git /aports
}

if [ ! -f /aports/scripts/mkimage.sh ]; then
	echo "== fetching aports (3.20-stable) =="
	fetch_aports
else
	echo "== reusing /aports =="
fi
# Hardening: apk index skips strict package-signature checks so that an APKINDEX is
# produced even when local/mirror packages and the index are occasionally inconsistent.
if ! grep -q "allow-untrusted" /aports/scripts/mkimg.base.sh; then
	sed -i "/--rewrite-arch/a \\        --allow-untrusted \\\\" /aports/scripts/mkimg.base.sh
fi
chmod -R a+rwX /aports

mkdir -p /home/builder/.mkimage
cp /work/mkimg/mkimg.aredn.sh    /home/builder/.mkimage/
cp /work/mkimg/genapkovl-aredn.sh /home/builder/.mkimage/
chmod +x /home/builder/.mkimage/*.sh
chown -R builder:builder /home/builder /aports

# abuild-keygen on this Alpine release writes .rsa keys (older releases used .privkey)
su builder -c 'HOME=/home/builder abuild-keygen -a -n' >/dev/null
PRIVKEY="$(ls /home/builder/.abuild/*.rsa /home/builder/.abuild/*.privkey 2>/dev/null | head -1)"
export PACKAGER_PRIVKEY="$PRIVKEY"
export PACKAGER_PUBKEY="${PRIVKEY%.privkey}.pub"

echo "== mkimage --profile aredn (v3.20 / x86_64) =="
mkdir -p /work/out
# mkimage runs as the non-root builder user; xorriso needs to create the ISO
# inside /work/out (a root-created 0755 dir would make it fail with
# "Failed to open device (a pseudo-drive): Permission denied")
chmod -R a+rwX /work/out
# give builder ownership of the staged firmware so cp -a does not warn
chown -R builder:builder /work/artifacts/$SRCDIR 2>/dev/null || true
su builder -c "HOME=/home/builder AREDN_EXTRA=$AREDN_EXTRA AREDN_VERSION=$AREDN_VERSION PACKAGER_PRIVKEY=$PRIVKEY PACKAGER_PUBKEY=${PRIVKEY%.privkey}.pub sh /aports/scripts/mkimage.sh --tag v3.20 --outdir /work/out --arch x86_64 --repository https://dl-cdn.alpinelinux.org/alpine/v3.20/main --profile aredn"

echo "== renaming and checksumming artifacts =="
ISO="aredn-${AREDN_VERSION}-x86_64-installer.iso"
SRC_ISO="$(ls /work/out/aredn-*.iso 2>/dev/null | head -1)"
[ -n "$SRC_ISO" ] || { echo "!! no ISO produced by mkimage"; exit 1; }
mv "$SRC_ISO" "/work/artifacts/$SRCDIR/$ISO"
(cd "/work/artifacts/$SRCDIR" && sha256sum "$ISO" > "$ISO.sha256")
echo "built: /work/artifacts/$SRCDIR/$ISO"
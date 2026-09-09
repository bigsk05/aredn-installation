#!/usr/bin/env bash
# publish.sh — publishes the freshly built ISOs under artifacts/<src>/ to GitHub Releases.
# Existing same-version assets are skipped upstream by fetch-aredn.sh; this only uploads new artifacts.
set -euo pipefail

REPO="${GITHUB_REPOSITORY:-$(git remote get-url origin | sed 's#.*:##; s#\.git$##')}"

shopt -s nullglob
for iso in artifacts/release/*.iso artifacts/nightly/*.iso; do
	dir="$(dirname "$iso")"
	tag="$(cat "$dir/TAG")"
	ver="$(cat "$dir/VERSION")"
	sha="$iso.sha256"
	echo "== publishing $iso -> $tag"
	# try to create the release first; if it already exists, upload with clobber instead
	if ! gh release create "$tag" "$iso" "$sha" \
		--repo "$REPO" \
		--title "AREDN $ver Installer ISO" \
		--notes "Auto-built AREDN x86_64 installer ISO.

- Firmware version: $ver
- Source: $(cat "$dir/SOURCE" 2>/dev/null || echo n/a)
- Usage: mount the ISO, boot, pick a disk interactively, type the disk path + YES.
  Headless panels can boot with the kernel option aredn.auto=1." >/dev/null 2>&1; then
		gh release upload "$tag" "$iso" "$sha" --clobber --repo "$REPO"
	fi
done
echo "== publishing done"
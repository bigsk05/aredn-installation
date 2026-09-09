#!/usr/bin/env bash
# fetch-aredn.sh — discovers and downloads the latest AREDN x86_64 firmware (BIOS + EFI),
# verifying sha256 checksums.
# Usage: fetch-aredn.sh <release|nightly> <outdir> <bindir>
#   release  : crawls downloads.arednmesh.org/releases/<maj>/<min>/<ver> for the max semver version
#   nightly  : uses the fixed /snapshots/targets/x86/64/ location
# If a GitHub Release for that version already exists (by tag), it skips and removes outdir.
set -euo pipefail

SRC="${1:?usage: fetch-aredn.sh <release|nightly> <outdir> <bindir>}"
OUT="$2"
BINDIR="$3"
BASE="https://downloads.arednmesh.org"
mkdir -p "$OUT"

latest_release_ver() {
	local maj min v best=""
	for maj in $(curl -sf "$BASE/releases/" | grep -oE 'href="[0-9]+/"' | sed 's/[^0-9]//g' | sort -V); do
		for min in $(curl -sf "$BASE/releases/$maj/" | grep -oE 'href="[0-9.]+/"' | sed 's/[^0-9.]//g' | sort -V); do
			for v in $(curl -sf "$BASE/releases/$maj/$min/" | grep -oE 'href="[0-9.]+/"' | sed 's/[^0-9.]//g' | sort -V); do
				best="$v"
			done
		done
	done
	echo "$best"
}

if [ "$SRC" = "nightly" ]; then
	PROF_URL="$BASE/snapshots/targets/x86/64/profiles.json"
	DIR_URL="$BASE/snapshots/targets/x86/64"
else
	VER="$(latest_release_ver)"
	[ -n "$VER" ] || { echo "!! cannot determine the latest release version"; exit 0; }
	maj="${VER%%.*}"; rest="${VER#*.}"; min="${rest%%.*}"
	PROF_URL="$BASE/releases/$maj/$min/$VER/targets/x86/64/profiles.json"
	DIR_URL="$BASE/releases/$maj/$min/$VER/targets/x86/64"
	echo "latest release: $VER"
fi

# parse profiles.json with python3 to get the version plus both image names
read -r VERSION_RAW NAME_B NAME_E <<<"$(python3 - "$PROF_URL" <<'PY'
import json, sys, urllib.request
d = json.load(urllib.request.urlopen(sys.argv[1]))
imgs = {i["type"]: i["name"] for i in d["profiles"]["generic"]["images"]}
print(d["version_number"], imgs.get("combined", ""), imgs.get("combined-efi", ""))
PY
)"
[ -n "$NAME_B" ] && [ -n "$NAME_E" ] || { echo "!! failed to parse profiles.json"; exit 1; }

TAG="aredn-iso-$VERSION_RAW"
if [ "${FORCE:-false}" != "true" ] && command -v gh >/dev/null 2>&1 && gh release view "$TAG" >/dev/null 2>&1; then
	echo "== $TAG already built, skipping (content unchanged)"
	rm -rf "$OUT"
	exit 0
fi

echo "== downloading firmware $VERSION_RAW ($SRC) =="
curl -sf "$DIR_URL/sha256sums" -o "$OUT/sha256sums"
curl -sf "$DIR_URL/$NAME_B" -o "$OUT/$NAME_B"
curl -sf "$DIR_URL/$NAME_E" -o "$OUT/$NAME_E"
# verify checksums (files are inside outdir)
(cd "$OUT" && sha256sum -c --ignore-missing sha256sums)
[ -f "$BINDIR/aredn-installer" ] && cp "$BINDIR/aredn-installer" "$OUT/"
echo "$VERSION_RAW" > "$OUT/VERSION"
echo "$TAG" > "$OUT/TAG"
echo "$DIR_URL" > "$OUT/SOURCE"
echo "== ready: $VERSION_RAW -> $OUT"
# AREDN x86_64 Installer ISO — daily automated build

Automatically packages the official [AREDN](https://www.arednmesh.org) x86_64 firmware into a
**bootable installer ISO**: boot from the ISO → **interactively pick a target disk (arrow-key menu +
type the disk path + type `YES` for double confirmation)** → `dd` the firmware straight to the disk →
optional rootfs expansion → automatic power-off. Built for KVM panels that can only mount an installer
ISO and cannot use a raw disk image directly (SolusVM/Virtualizor/Proxmox, Vultr custom ISO, DigitalOcean
custom ISO, etc.).

A GitHub Actions workflow runs **daily at 02:20 UTC** and tracks **both the latest stable release and the
latest nightly**: unchanged content is skipped, and each new version gets its own ISO published to Releases.

## Layout

```
.github/workflows/build-iso.yml   # daily cron + manual trigger; release & nightly tracks
installer/                        # Go interactive installer (static binary)
  main.go                         #   orchestration: boot-mode detect / image pick / confirm / write / poweroff
  disks.go                        #   /sys/block enumeration + candidate filter
  ui.go                           #   arrow-key menu + double confirmation
  write.go                        #   gzip->block device dd with progress; optional rootfs grow
mkimg/
  mkimg.aredn.sh                  #   Alpine mkimage custom profile
  genapkovl-aredn.sh              #   live overlay (hostname / world packages / installer / auto-start)
scripts/
  fetch-aredn.sh                  #   discover latest release/nightly, download + sha256 verify
  build.sh                        #   build the ISO with mkimage inside an Alpine container
  publish.sh                      #   publish to GitHub Releases
```

## Usage

1. Push this repository (default branch).
2. Actions → `Build AREDN x86_64 installer ISO` → `Run workflow` (initial manual run).
3. It then runs daily; artefacts land on the **Releases** page, named `aredn-<version>-x86_64-installer.iso`.

### Installing on a provider (panel)

1. Download the ISO, attach it to the VM's CD-ROM, and set boot-from-CD (BIOS or UEFI both work; the ISO is hybrid).
2. Boot (use the VNC/NoVNC console). **The installer TUI starts automatically on boot, no login needed**:
   - Arrow-key to the disk → type the disk path → type `YES` → it writes.
   - To exit: press `Ctrl-C` to get a root shell (you can run `aredn-installer` again);
     to disable auto-start: boot with `aredn.autorun=0`.
   - Headless panels (no console): add `aredn.auto=1` to auto-pick the first candidate disk (confirmation is still required).
3. After the write completes the machine **powers off automatically after 15 seconds** (press any key + Enter to
   cancel and stay running) → **detach the ISO in the panel**, then power the machine back on.
4. The node defaults to `192.168.1.1` (`localnode.local.mesh`); finish the setup per the AREDN documentation.

### Kernel cmdline options (`aredn.*`)

| Option | Meaning |
|---|---|
| `aredn.auto=1` | Auto-select the first candidate disk (disk path + YES confirmation still required) |
| `aredn.auto=yes` | Fully automatic write, skips confirmation (dangerous; consoles-less panels only) |
| `aredn.autorun=0` | Disable auto-start of the installer on boot (default: enabled; Ctrl-C drops to a shell) |
| `aredn.image=efi` / `bios` | Force the EFI / BIOS firmware image (default: auto-pick by boot mode) |
| `aredn.grow=1` | Expand rootfs offline to fill the disk after writing |

## Local verification

```sh
# 1) build the installer
cd installer && CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o aredn-installer .

# 2) build the ISO inside an Alpine container (requires docker)
docker run --rm -v "$PWD":/work -w /work alpine:3.20 sh scripts/build.sh release

# 3) qemu smoke test: should reach the login/installer prompt
qemu-system-x86_64 -cdrom artifacts/release/aredn-*-installer.iso -m 1024 -boot d -nographic
```

## Notes & caveats

- **Firmware trust**: every build verifies against `downloads.arednmesh.org`'s `sha256sums`.
- **Image layout** (verified on 4.26.7.0): BIOS image = MBR + GRUB + ext4 `kernel` (16 MiB) + ext4 `rootfs` (104 MiB);
  EFI image = GPT + FAT16 ESP (contains `\EFI\BOOT\BOOTX64.EFI`) + ext4 `rootfs`. Each image boots standalone
  after `dd`; the installer picks the right one via `/sys/firmware/efi`.
- **rootfs growth**: the official rootfs is a fixed 104 MiB; `aredn.grow=1` grows it offline (needs `gptfdisk`/`e2fsprogs`,
  both baked into the ISO). Best-effort feature — validate in qemu and on a real provider before production.
- **Secure Boot**: neither the images nor the ISO are signed; disable Secure Boot on the VM (most KVM panels default to off).
- **Memory**: the Alpine live system unpacks packages into RAM, so give the VM ≥ 512 MiB.
- The menu TUI is implemented in Go (`golang.org/x/term`), statically compiled with zero runtime dependencies;
  swapping to Rust would also work — the binary is just copied into the ISO.
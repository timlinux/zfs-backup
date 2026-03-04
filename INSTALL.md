# Installation Guide

## Quick Install (Binary Download)

Download the pre-built binary for your platform from the [releases page](https://github.com/timlinux/zfs-backup/releases):

```bash
# Linux x86_64
curl -L https://github.com/timlinux/zfs-backup/releases/latest/download/zfs-backup-linux-amd64 -o zfs-backup
chmod +x zfs-backup
sudo mv zfs-backup /usr/local/bin/

# Linux ARM64
curl -L https://github.com/timlinux/zfs-backup/releases/latest/download/zfs-backup-linux-arm64 -o zfs-backup
chmod +x zfs-backup
sudo mv zfs-backup /usr/local/bin/

# macOS Intel
curl -L https://github.com/timlinux/zfs-backup/releases/latest/download/zfs-backup-darwin-amd64 -o zfs-backup
chmod +x zfs-backup
sudo mv zfs-backup /usr/local/bin/

# macOS Apple Silicon
curl -L https://github.com/timlinux/zfs-backup/releases/latest/download/zfs-backup-darwin-arm64 -o zfs-backup
chmod +x zfs-backup
sudo mv zfs-backup /usr/local/bin/
```

## NixOS / Nix

### Using Flakes (recommended)

Run directly:
```bash
nix run github:timlinux/zfs-backup
```

Install to profile:
```bash
nix profile install github:timlinux/zfs-backup
```

### NixOS Module

Add to your `flake.nix`:
```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    zfs-backup.url = "github:timlinux/zfs-backup";
  };

  outputs = { self, nixpkgs, zfs-backup }: {
    nixosConfigurations.myhost = nixpkgs.lib.nixosSystem {
      modules = [
        zfs-backup.nixosModules.default
        {
          programs.zfs-backup.enable = true;
        }
      ];
    };
  };
}
```

## Arch Linux (AUR)

### Using an AUR helper (yay, paru, etc.)

```bash
yay -S zfs-backup
# or
paru -S zfs-backup
```

### Manual installation

```bash
git clone https://github.com/timlinux/zfs-backup.git
cd zfs-backup/packaging/aur
makepkg -si
```

## Debian / Ubuntu

### Build from packaging files

```bash
git clone https://github.com/timlinux/zfs-backup.git
cd zfs-backup

# Copy debian packaging files
mkdir -p debian
cp packaging/deb/* debian/

# Build the package
dpkg-buildpackage -us -uc -b

# Install
sudo dpkg -i ../zfs-backup_1.2.0-1_*.deb
sudo apt-get install -f  # Install dependencies
```

### Dependencies

```bash
sudo apt-get install zfsutils-linux sanoid udisks2
```

## Fedora / RHEL / CentOS

### Build RPM

```bash
# Install build dependencies
sudo dnf install golang rpm-build

# Clone and build
git clone https://github.com/timlinux/zfs-backup.git
cd zfs-backup

# Create source tarball
tar czf zfs-backup-1.2.0.tar.gz --transform 's,^,zfs-backup-1.2.0/,' \
    *.go go.mod go.sum vendor/ LICENSE README.md

# Build RPM
rpmbuild -tb zfs-backup-1.2.0.tar.gz

# Install
sudo rpm -i ~/rpmbuild/RPMS/x86_64/zfs-backup-1.2.0-1.*.rpm
```

### Dependencies

```bash
sudo dnf install zfs sanoid udisks2
```

## Snap

```bash
# Build locally
cd packaging/snap
snapcraft

# Install
sudo snap install zfs-backup_1.2.0_amd64.snap --classic --dangerous
```

Or wait for publication to the Snap Store:
```bash
sudo snap install zfs-backup --classic
```

## Flatpak

> **Note:** Flatpak sandboxing is not ideal for ZFS tools which need direct access
> to block devices and ZFS kernel modules. Consider using native packages instead.

```bash
# Build
cd packaging/flatpak
flatpak-builder --user --install build com.kartoza.ZfsBackup.yml

# Run
flatpak run com.kartoza.ZfsBackup
```

## Building from Source

### Prerequisites

- Go 1.22 or later
- ZFS utilities (`zfs`, `zpool` commands)
- syncoid (from sanoid package)
- udisks2 (optional, for USB power management)

### Build

```bash
git clone https://github.com/timlinux/zfs-backup.git
cd zfs-backup
go build -ldflags="-s -w" -o zfs-backup .
sudo mv zfs-backup /usr/local/bin/
```

## Verifying Downloads

SHA256 checksums are provided with each release:

```bash
# Download checksum file
curl -L https://github.com/timlinux/zfs-backup/releases/latest/download/checksums-sha256.txt -o checksums.txt

# Verify binary
sha256sum -c checksums.txt --ignore-missing
```

## Dependencies

| Dependency | Required | Purpose |
|------------|----------|---------|
| zfs/zpool | Yes | ZFS filesystem commands |
| syncoid | Yes | Efficient incremental backups |
| udisks2 | Recommended | USB drive power management |

## Post-Installation

The tool requires root privileges or ZFS delegation to function:

```bash
# Run with sudo
sudo zfs-backup

# Or configure ZFS delegation for your user
sudo zfs allow -u $USER create,destroy,snapshot,mount,send,receive,hold,release POOLNAME
```

---

Made with <3 by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/timlinux/zfs-backup)

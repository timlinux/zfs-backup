# Packaging

This guide covers installing zfs-backup using various package managers and building packages from source.

## Pre-built Binaries

The easiest installation method is downloading pre-built binaries from the [releases page](https://github.com/timlinux/zfs-backup/releases).

=== "Linux x86_64"
    ```bash
    curl -L https://github.com/timlinux/zfs-backup/releases/latest/download/zfs-backup-linux-amd64 -o zfs-backup
    chmod +x zfs-backup
    sudo mv zfs-backup /usr/local/bin/
    ```

=== "Linux ARM64"
    ```bash
    curl -L https://github.com/timlinux/zfs-backup/releases/latest/download/zfs-backup-linux-arm64 -o zfs-backup
    chmod +x zfs-backup
    sudo mv zfs-backup /usr/local/bin/
    ```

=== "macOS Intel"
    ```bash
    curl -L https://github.com/timlinux/zfs-backup/releases/latest/download/zfs-backup-darwin-amd64 -o zfs-backup
    chmod +x zfs-backup
    sudo mv zfs-backup /usr/local/bin/
    ```

=== "macOS Apple Silicon"
    ```bash
    curl -L https://github.com/timlinux/zfs-backup/releases/latest/download/zfs-backup-darwin-arm64 -o zfs-backup
    chmod +x zfs-backup
    sudo mv zfs-backup /usr/local/bin/
    ```

### Verifying Downloads

```bash
curl -L https://github.com/timlinux/zfs-backup/releases/latest/download/checksums-sha256.txt -o checksums.txt
sha256sum -c checksums.txt --ignore-missing
```

## NixOS / Nix

### Using Flakes (Recommended)

Run directly without installing:

```bash
nix run github:timlinux/zfs-backup
```

Install to your profile:

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

### Using an AUR Helper

```bash
yay -S zfs-backup
# or
paru -S zfs-backup
```

### Manual Build

```bash
git clone https://github.com/timlinux/zfs-backup.git
cd zfs-backup/packaging/aur
makepkg -si
```

## Debian / Ubuntu

### Build from Source

```bash
git clone https://github.com/timlinux/zfs-backup.git
cd zfs-backup

# Copy debian packaging files
mkdir -p debian
cp packaging/deb/* debian/

# Build the package
dpkg-buildpackage -us -uc -b

# Install
sudo dpkg -i ../zfs-backup_*.deb
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
sudo rpm -i ~/rpmbuild/RPMS/x86_64/zfs-backup-*.rpm
```

### Dependencies

```bash
sudo dnf install zfs sanoid udisks2
```

## Snap

```bash
cd packaging/snap
snapcraft
sudo snap install zfs-backup_*.snap --classic --dangerous
```

!!! warning "Classic Confinement"
    ZFS backup requires classic confinement to access block devices and ZFS commands.

## Flatpak

!!! warning "Not Recommended"
    Flatpak sandboxing is not ideal for ZFS tools which need direct access to block devices and kernel modules. Consider using native packages instead.

```bash
cd packaging/flatpak
flatpak-builder --user --install build com.kartoza.ZfsBackup.yml
flatpak run com.kartoza.ZfsBackup
```

## Building from Source

### Prerequisites

- Go 1.22 or later
- ZFS utilities
- syncoid (from sanoid)

### Build

```bash
git clone https://github.com/timlinux/zfs-backup.git
cd zfs-backup
go build -ldflags="-s -w" -o zfs-backup .
sudo mv zfs-backup /usr/local/bin/
```

## Dependencies Summary

| Dependency | Required | Purpose |
|------------|----------|---------|
| zfs/zpool | Yes | ZFS filesystem commands |
| syncoid | Yes | Efficient incremental backups |
| udisks2 | Recommended | USB drive power management |

---

Made with <3 by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/timlinux/zfs-backup)

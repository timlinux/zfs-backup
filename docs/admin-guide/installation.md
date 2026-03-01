# Installation

This guide covers various methods to install Kartoza ZFS Backup Tool.

## NixOS Module (Recommended)

The easiest way to install on NixOS is using the provided flake module.

### Step 1: Add to flake.nix

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    zfs-backup.url = "github:kartoza/zfs-backup";
  };

  outputs = { self, nixpkgs, zfs-backup, ... }: {
    nixosConfigurations.myhost = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        # Import the module
        zfs-backup.nixosModules.default
        # Add the overlay so pkgs.zfs-backup is available
        { nixpkgs.overlays = [ zfs-backup.overlays.default ]; }

        ./configuration.nix
      ];
    };
  };
}
```

### Step 2: Enable in configuration.nix

```nix
{ config, pkgs, ... }:
{
  # Enable the service (adds package to systemPackages)
  services.zfs-backup.enable = true;
}
```

### Step 3: Rebuild

```bash
sudo nixos-rebuild switch
```

---

## Direct Package Installation (NixOS)

If you don't need the module features, just add the overlay:

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    zfs-backup.url = "github:kartoza/zfs-backup";
  };

  outputs = { self, nixpkgs, zfs-backup, ... }: {
    nixosConfigurations.myhost = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        { nixpkgs.overlays = [ zfs-backup.overlays.default ]; }
        ./configuration.nix
      ];
    };
  };
}
```

Then in `configuration.nix`:

```nix
{ config, pkgs, ... }:
{
  environment.systemPackages = [ pkgs.zfs-backup ];
}
```

---

## Run Without Installing

Try the tool without installing:

```bash
nix run github:kartoza/zfs-backup
```

---

## Building from Source

### Prerequisites

- Go 1.21 or later
- ZFS utilities installed
- syncoid (from sanoid package)

### Build Steps

```bash
# Clone the repository
git clone https://github.com/kartoza/zfs-backup
cd zfs-backup

# Using Nix (recommended)
nix build
./result/bin/zfs-backup

# Or using Go directly
go build -o zfs-backup .
./zfs-backup
```

### Using the Makefile

```bash
# Build
make build

# Run
make run

# Clean
make clean

# Build for all platforms
make build-all
```

---

## System Dependencies

Ensure these are installed on your system:

| Dependency | Purpose |
|------------|---------|
| ZFS | Core filesystem operations |
| syncoid | Efficient snapshot synchronization |
| udisks2 | USB drive power management |

### Installing on Debian/Ubuntu

```bash
sudo apt install zfsutils-linux sanoid udisks2
```

### Installing on Arch Linux

```bash
sudo pacman -S zfs-utils sanoid udisks2
```

### Installing on NixOS

These are typically available when ZFS is enabled:

```nix
{ config, pkgs, ... }:
{
  boot.supportedFilesystems = [ "zfs" ];
  environment.systemPackages = with pkgs; [
    sanoid
    udisks2
  ];
}
```

---

## Verifying Installation

After installation, verify everything works:

```bash
# Check the tool runs
zfs-backup --help

# Check ZFS is available
zfs version

# Check syncoid is available
syncoid --version
```

---

Made with :heart: by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/kartoza/zfs-backup)

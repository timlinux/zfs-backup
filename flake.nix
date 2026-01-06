{
  description = "Beautiful TUI for managing ZFS backups with Bubble Tea";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages = {
          zfs-backup = pkgs.callPackage ./package.nix { };
          default = self.packages.${system}.zfs-backup;
        };

        apps = {
          zfs-backup = {
            type = "app";
            program = "${self.packages.${system}.zfs-backup}/bin/zfs-backup";
          };
          default = self.apps.${system}.zfs-backup;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            gotools
            go-tools
            golangci-lint
            zfs
            sanoid
            udisks2
          ];

          shellHook = ''
            echo "🗄️  ZFS Backup Development Environment"
            echo ""
            echo "Available commands:"
            echo "  go build       - Build the application"
            echo "  go run .       - Run the application"
            echo "  go test        - Run tests"
            echo "  nix build      - Build with Nix"
            echo "  nix run        - Run with Nix"
            echo ""
          '';
        };

        formatter = pkgs.nixfmt-rfc-style;
      }
    );
}

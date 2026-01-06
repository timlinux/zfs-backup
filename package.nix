{
  lib,
  buildGoModule,
  makeWrapper,
  zfs,
  sanoid,
  udisks2,
}:

buildGoModule rec {
  pname = "zfs-backup";
  version = "1.0.0";

  src = ./.;

  vendorHash = null;

  # Allow network access during build for Go modules
  proxyVendor = true;

  # Runtime dependencies
  buildInputs = [
    zfs
    sanoid
    udisks2
  ];

  # Ensure runtime dependencies are in PATH
  postInstall = ''
    wrapProgram $out/bin/zfs-backup \
      --prefix PATH : ${
        lib.makeBinPath [
          zfs
          sanoid
          udisks2
        ]
      }
  '';

  nativeBuildInputs = [ makeWrapper ];

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];

  meta = with lib; {
    description = "Beautiful TUI for managing ZFS backups with Bubble Tea";
    homepage = "https://github.com/timlinux/zfs-backup";
    license = licenses.mit;
    maintainers = [ ];
    platforms = platforms.linux;
    mainProgram = "zfs-backup";
  };
}

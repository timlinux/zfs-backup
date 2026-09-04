{
  lib,
  buildGoModule,
  makeWrapper,
  zfs,
  sanoid,
  udisks2,
  # Short git SHA this build came from, shown beside the version so a running
  # binary can be matched to its source. The flake passes the real value.
  rev ? "unknown",
}:

buildGoModule rec {
  pname = "zfs-backup";
  version = builtins.replaceStrings ["\n"] [""] (builtins.readFile ./VERSION);

  src = ./.;

  vendorHash = null;  # Use vendored dependencies from vendor/ directory

  # Runtime dependencies
  buildInputs = [
    zfs
    sanoid
    udisks2
  ];

  # Ensure runtime dependencies are in PATH (pandoc/texlive optional for PDF reports)
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
    "-X main.appVersion=${version}"
    "-X main.appCommit=${rev}"
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

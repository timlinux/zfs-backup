{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.zfs-backup;
in
{
  options.services.zfs-backup = {
    enable = mkEnableOption "ZFS backup tool";

    package = mkOption {
      type = types.package;
      default = pkgs.zfs-backup;
      defaultText = literalExpression "pkgs.zfs-backup";
      description = "The zfs-backup package to use.";
    };
  };

  config = mkIf cfg.enable {
    environment.systemPackages = [ cfg.package ];
  };
}

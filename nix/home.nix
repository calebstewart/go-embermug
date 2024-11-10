{self, ...}: {config, lib, pkgs, osConfig, ...}:
let
  cfg = config.services.embermug;
in {
  options.services.embermug = {
    enable = lib.mkEnableOption "Ember Mug SystemD Service";

    socketPath = lib.mkOption {
      description = "Path of the Unix Domain Socket listener";
      default = "/tmp/embermug.sock";
    };
    
    deviceAddress = lib.mkOption {
      description = "MAC address of ember mug";
    };

    package = lib.mkOption {
      default = self.packages.${pkgs.system}.default;
    };

    waybarClientCommand = lib.mkOption {
      description = "Command used to invoke a waybar client for the configured service";
    };
  };

  config = lib.mkIf cfg.enable {
    services.embermug.waybarClientCommand = lib.mkForce (lib.escapeShellArgs [
      (lib.getExe cfg.package)
      "waybar"
      "--socket" cfg.socketPath
    ]);

    systemd.user.sockets.embermug = {
      Unit = {
        Description = "Ember Mug Client Socket";
        PartOf = "embermug.service";
      };

      Socket.ListenStream = cfg.socketPath;

      Install = {
        WantedBy = ["sockets.target"];
      };
    };

    systemd.user.services.embermug = {
      Unit = {
        Description = "Ember Mug Service";
        After = ["network.target" "embermug.socket"];
        Requires = ["embermug.socket"];
      };

      Service = {
        Type = "simple";
        ExecStart = lib.escapeShellArgs [
          (lib.getExe cfg.package)
          "service"
          "--socket" cfg.socketPath
          cfg.deviceAddress
        ];
      };

      Install = {
        WantedBy = ["default.target"];
      };
    };
  };
}

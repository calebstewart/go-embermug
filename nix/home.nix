{self, ...}: {config, lib, pkgs, osConfig, ...}:
let
  cfg = config.programs.embermug;
in {
  options.programs.embermug = {
    enable = lib.mkEnableOption "Ember Mug SystemD Service";
    enableNotifications = lib.mkEnableOption "Send Desktop Notifications when reaching ideal temperature";

    systemd = {
      enable = lib.mkEnableOption "Create SystemD Service";
      socket-activation = lib.mkEnableOption "Utilize SystemD Socket Activation";
      target = lib.mkOption {
        description = "Name of the target used for the Install.WantedBy field of the systemd service";
        type = lib.types.str;
        default = "default.target";
      };
    };

    settings = lib.mkOption {
      description = "Free-form settings written to embermug configuration file in TOML format";
      type = lib.types.attrs;
      default = {
        socket-path = "/tmp/embermug.sock";
      };
      example = {
        socket-path = "/tmp/embermug.sock";

        service = {
          device-address = "AA:BB:CC:DD:EE:FF";
          enable-notifications = true;
        };

        waybar = 
        let
          tooltip = "Battery: {{ .Battery.Charge }}% ({{if not .Battery.Charging}}dis{{end}}charging)";
          active = {
            inherit tooltip;
            text = "{{ .State }} ({{ toFahrenheit .Current }}F/{{ toFahrenheit .Target }}F)";
          };
        in {
          disconnected.text = "Disconnected";

          default = {
            inherit tooltip;
            text = "{{ .State }}";
          };

          state.heating = active;
          state.cooling = active;
          
        };
      };
    };

    waybar = {
      enable = lib.mkEnableOption "Waybar Integration";
      block-name = lib.mkOption {
        description = "Name of the block in waybar configuration";
        default = "custom/embermug";
        type = lib.types.str;
      };
    };

    package = lib.mkOption {
      default = self.packages.${pkgs.system}.default;
      type = lib.types.package;
    };
  };

  config = lib.mkIf cfg.enable {
    xdg.configFile."embermug/config.toml" = 
    let
      toml = pkgs.formats.toml {};
    in {
      enable = true;
      source = toml.generate "config.toml" cfg.settings;
    };

    xdg.configFile."waybar/embermug.json" = {
      enable = cfg.waybar.enable;

      # This is a stupid hack because of two bugs. First, using the unicode
      # codepoint for the coffee mug in Vim seems to break something within
      # the Nix LSP, and render it useless. Additionally, you can't get
      # escaped code points through `builtins.toJSON` correctly. So, I
      # output an escape escaped code point, and then remove the extra
      # escape with `builtins.replaceStrings`. This is fucking stupid.
      text = builtins.replaceStrings ["\\u"] ["\u"] (builtins.toJSON {
        "${cfg.waybar.block-name}" = {
          exec = lib.escapeShellArgs [
            (lib.getExe cfg.package)
            "waybar"
          ];
          format = "{icon}  {}";
          format-icons = "\\uf0f4";
          return-type = "json";
          restart-interval = 1;
        };
      });
    };

    systemd.user = lib.mkIf cfg.systemd.enable {
      sockets.embermug = lib.mkIf cfg.systemd.socket-activation {
        Unit = {
          Description = "Ember Mug Client Socket";
          PartOf = "embermug.service";
        };

        # Initialize the listen stream either from the settings or with the default path
        Socket.ListenStream = lib.attrByPath ["socket-path"] "/tmp/embermug.sock" cfg.settings;

        Install = {
          WantedBy = [cfg.systemd.target];
        };
      };

      services.embermug = {
        Unit = {
          Description = "Ember Mug Service";
          After = ["network.target"] ++ (lib.lists.optional cfg.systemd.socket-activation "embermug.socket");
          Requires = lib.lists.optional cfg.systemd.socket-activation "embermug.socket";
        };

        Service = {
          Type = "simple";
          ExecStart = lib.escapeShellArgs [
            (lib.getExe cfg.package)
            "service"
          ];
        };

        Install = {
          WantedBy = [cfg.systemd.target];
        };
      };
    };
  };
}

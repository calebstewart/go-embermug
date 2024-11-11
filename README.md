# Ember Mug Golang Client
This repository implements a Golang client for the [Ember Mug]. This project is made possible by the
amazing work of @orlopau over at their [ember-mug] repo! Many thanks to their initial RE work.

The root of this project is an importable client library for the Ember Mug written on top of the
[TinyGO go-bluetooth] module. The `cli/` directory implements a client and server binary intended
to be used for adding your ember mug to [Waybar]. The server takes control of the Ember Mug
bluetooth device, and registers event listeners. It also listens on a Unix socket for clients.
The waybar client entrypoint connects to the unix socket and writes mug updates to standard output
in accordance with the waybar custom block format.

The server is setup to work with SystemD Socket Activation as well. If it is invoked without socket
activation, it will open a unix socket at the path provided by the `--socket` argument (defaults to
`/tmp/embermug.sock`).

Additionally, if the `--enable-notifications` argument is provided, then it will create a desktop
notification when the mug reaches the stable target temperature.

## Configuration
Both the service and waybar entrypoints read the same configuration file. It is a TOML file, and
the general structure is:

```toml
socket-path = "/path/to/socket"

[service]
device-address = "aa:bb:cc:dd:ee:ff"
enable-notifications = true

[waybar.disconnected]
text = "Disconnected"
tooltip = "We are not connected :("

[waybar.default]
text = "Text/block info when none of the below states match"

[waybar.state.cooling]
text = "{{ .State }} Change stuff for individual states"
```

The waybar configuration allows you to specify the exact text, tooltip, alt-text, class, and percentage
for the waybar block. The names of those fields are the same as for the JSON waybar custom block.
For `text`, `tooltip`, `alt`, and `class` the value is a Golang `text/template` template string.
The available fields can be found in [service.go](./service/service.go) in the struct `State`. For
example to write the mug state, the current temperature in fahrenheit, and the battery percentage in the main
text of the block, you can use:

```toml
[waybar.default]
text = "{{ .State }} {{ toFahrenheit .Current }} {{ .Battery.Charge }}%"
```

The functions `toFahrenheit` and `toCelsius` are provided to format the temperatures appropriately.

If no `waybar.state.*` values are provided, then defaults will be loaded for `waybar.start.cooling` and
`waybar.state.heating`. Similarly, if `waybar.default` or `waybar.disconnected` are not provided, a
default will be loaded. The defaults are functionally equivalent to the following:

```toml
socket-path = "/tmp/embermug.sock"

[service]
enable-notifications = false

[waybar.disconnected]
text = "Disconnected"

[waybar.default]
text = "{{ .State }}"
tooltip = "Battery: {{ .Battery.Charge }}% ({{if not .Battery.Charging}}dis{{end}}charging)"

[waybar.state.heating]
text = "{{ .State }} ({{ toFahrenheit .Current }}F/{{ toFahrenheit .Target }}F)"
tooltip = "Battery: {{ .Battery.Charge }}% ({{if not .Battery.Charging}}dis{{end}}charging)"

[waybar.state.cooling]
text = "{{ .State }} ({{ toFahrenheit .Current }}F/{{ toFahrenheit .Target }}F)"
tooltip = "Battery: {{ .Battery.Charge }}% ({{if not .Battery.Charging}}dis{{end}}charging)"
```

## Installation (NixOS w/ Home Manager)
This repository is a Nix Flake which exports a `homeModules.default` output which is a Home Manager
module. If you use the module, you can configure the service like this:

```nix
programs.embermug = {
  enable                    = true; # Generate configuration
  systemd.enable            = true; # Install a systemd service wanted-by [systemd.target]
  systemd.socket-activation = true; # Install a systemd socket and have it activate the service
  systemd.target            = "default.target" # this is the default
  waybar.enable             = true; # Install $XDG_CONFIG_HOME/waybar/embermug.json containing a block configuration
  waybar.block-name         = "custom/embermug"; # this is the default

  # Free-form settings written to $XDG_CONFIG_HOME/embermug/config.toml
  # This is equivalent to the default with no configuration. You must set
  # 'service.device-address' for the service to function.
  settings = {
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
```

Within your Waybar config, you can import the new configuration file, and then use the
custom block (by default named "custom/embermug").

```nix
programs.waybar = {
  enable = true;

  settings = [{
    include = ["${xdg.configHome}/waybar/embermug.json"];

    modules-right = [
      "custom/embermug"
    ];
  }];
};
```

## Installation (Other)
You need to run the service somehow. I like using SystemD socket activation, but you can execute it
however you like. You just must ensure it is running prior to Waybar starting, and if it exits, it
is restarted. Then, in your Waybar config, create a block like:

```json
{
  "custom/embermug": {
    "exec": "/path/to/embermug waybar",
    "format": "{icon} {}",
    "format-icons": "",
    "return-type": "json",
    "restart-interval": 5,
  }
}
```

The service and waybar clients both load their configuration file from the XDG configuration directories
under `embermug/config.toml` (e.g. `~/.config/embermug/config.toml` for standard user account).

[Ember Mug]: https://ember.com/products/ember-mug-2
[ember-mug]: https://github.com/orlopau/ember-mug
[TinyGO go-bluetooth]: https://github.com/tinygo-org/bluetooth
[Waybar]: https://github.com/Alexays/Waybar

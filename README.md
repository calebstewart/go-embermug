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

## Installation (NixOS w/ Home Manager)
This repository is a Nix Flake which exports a `homeModules.default` output which is a Home Manager
module. If you use the module, you can configure the service like this:

```nix
services.embermug = {
  enable    = true;
  socketPath  = "/path/to/socket"; # optional, defaults to '/tmp/embermug.sock'
  deviceAddress = "AA:BB:CC:DD:EE:FF";
  package     = my-embermug-pkg; # optional, defaults to package built from flake
};
```

Within your Waybar config, you can use `config.services.embermug.waybarClientCommand` as the `exec`
field of your custom block. It will automatically set to a command line string which invokes the
`embermug` waybar entrypoint appropriately for your service config. My configuration looks like:

```nix
programs.waybar = {
  enable = true;

  settings = [{
    "custom/embermug" = {
      exec = config.services.embermug.waybarClientCommand;
      format = "{icon}  {}";
      format-icons = "";
      return-type = "json";
      restart-interval = 1;
    };
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
    "exec": "/path/to/embermug waybar --socket /path/to/socket.sock",
    "format": "{icon} {}",
    "format-icons": "",
    "return-type": "json",
    "restart-interval": 5,
  }
}
```

[Ember Mug]: https://ember.com/products/ember-mug-2
[ember-mug]: https://github.com/orlopau/ember-mug

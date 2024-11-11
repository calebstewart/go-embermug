{
  description = "Golang Ember Mug Service";

  # Nixpkgs / NixOS version to use.
  inputs.nixpkgs.url = "nixpkgs/nixos-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";

  outputs = {self, nixpkgs, flake-utils, ...}: flake-utils.lib.eachDefaultSystem (system:
  let
    pkgs = import nixpkgs { inherit system; };
    lib = nixpkgs.lib;
  in
  {
    packages.default = pkgs.buildGoModule {
      pname = "go-embermug";
      version = "0.0.1";
      src = ./.;
      subPackages = ["./cli"];
      vendorHash = "sha256-AVJ3h3w64sRVHZSgiiCDVlQNqWzZrXEIEmFd/xASihU=";
      postInstall = "mv $out/bin/cli $out/bin/embermug";

      meta = {
        description = "Ember Mug Service and Waybar Custom Block";
        homepage = "https://github.com/calebstewart/go-embermug";
        license = lib.licenses.mit;
        mainProgram = "embermug";
      };
    };
    defaultPackage = self.packages.${system}.default;

    devShells.default = pkgs.mkShell {
      buildInputs = with pkgs; [
        go
        gopls
        gotools
        go-tools
        delve
        golangci-lint
        gotestsum
      ];
    };
  }) // {
    overlays.default = (final: prev: {
      go-embermug = self.packages.default;
    });

    homeModules.default = import ./nix/home.nix {
      inherit self;
    };
  };
}

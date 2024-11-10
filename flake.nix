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
      vendorHash = lib.fakeHash;
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
  });
}

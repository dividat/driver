{
  description = ''
    Provides cross-compiled binaries of Dividat driver for Windows and Linux,
    and a development shell for Linux and macOS.
  '';
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/24.11";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        crossBuild = import ./nix/crossBuild.nix {
          inherit pkgs;
        };
      in
      {
        devShells = {
          crossBuild = {
            inherit (crossBuild) x86_64-linux;
            inherit (crossBuild) x86_64-windows;
            inherit (crossBuild) darwin;
          };
          default = import ./nix/devShell.nix {
            inherit pkgs;
          };
        };
      }
    );
}


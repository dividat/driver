{
  description = ''
    Provides cross-compiled binaries of Dividat driver for Windows and Linux,
    and a development shell for Linux and macOS.
  '';
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/24.11";
    # only used for crossBuilding on darwin, since 24.11 is broken
    nixpkgs2505.url = "github:nixos/nixpkgs/25.05";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, nixpkgs2505, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };

        crossBuild = import ./nix/crossBuild.nix {
          inherit pkgs;
        };

        crossBuildDarwin = import ./nix/crossBuild.nix {
          pkgs = import nixpkgs2505 { inherit system; };
        };
      in
      {
        devShells = {
          crossBuild = {
            inherit (crossBuild) x86_64-linux;
            inherit (crossBuild) x86_64-windows;
            inherit (crossBuildDarwin) darwin;
          };
          default = import ./nix/devShell.nix {
            inherit pkgs;
          };
        };
      }
    );
}


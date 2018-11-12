{ pkgs, goPath, goPackagePath }:
let
  crossStdenv = system: 
  ((import ./nix/nixpkgs.nix) { 
    crossSystem = { config = system; }; 
  }).stdenv;

  crossPkgs = system: 
  ((import ./nix/nixpkgs.nix) { 
    crossSystem = { config = system; }; 
  }).pkgs;

in
  {
    linux = 
    (crossStdenv "x86_64-unknown-linux-musl").mkDerivation {
      name = "dividat-driver";

      src = ./src;

      configurePhase = ''
        echo $CC
        export GOPATH=${goPath}:"$NIX_BUILD_TOP"
        export STATIC_BUILD=1
      '';

      buildPhase = ''
        mkdir -p $out
        go build -o $out/dividat-driver ${goPackagePath}
      '';

      installPhase = ''
        echo hello
      '';

      nativeBuildInputs = with pkgs; [
        go_1_9
        pkgconfig
      ];

      buildInputs = with crossPkgs "x86_64-unknown-linux-musl"; [ 
        ((import ./nix/pcsclite) {inherit stdenv fetchFromGitHub pkgconfig autoconf automake libtool flex python perl;}) 

        # Required for building go dependencies
        autoconf automake libtool flex
      ];

    };


    windows = (crossStdenv "x86_64-pc-mingw32").mkDerivation {
      name = "dividat-driver";

      src = ./src;

      configurePhase = ''
        echo $CC
        export GOPATH=${goPath}:"$NIX_BUILD_TOP"
      '';

      buildPhase = ''
        mkdir -p $out
        GOOS=windows GOARCH=amd64 go build -o $out/dividat-driver ${goPackagePath}
      '';

      installPhase = ''
        echo hello
      '';

      nativeBuildInputs = with pkgs; [
        go_1_9
      ];

      buildInputs = with crossPkgs "x86_64-pc-mingw32"; [ 
        windows.mingw_w64_pthreads
      ];

    };

  }

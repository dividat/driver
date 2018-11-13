{ pkgs
, src
, version
, channel
, releaseUrl
, goPath }:
let
  crossStdenv = system: 
  ((import ./nix/nixpkgs.nix) { 
    crossSystem = { config = system; }; 
  }).stdenv;

  crossPkgs = system: 
  ((import ./nix/nixpkgs.nix) { 
    crossSystem = { config = system; }; 
  }).pkgs;

  configurePhase = ''
    export GOPATH=${goPath}:`pwd`
    export STATIC_BUILD=1

    export VERSION=${version}
    export CHANNEL=${channel}
    export RELEASE_URL=${releaseUrl}
  '';

in
  {
    linux = 
    (crossStdenv "x86_64-unknown-linux-musl").mkDerivation {
      name = "dividat-driver";

      inherit src;

      configurePhase = configurePhase + ''
        export GOOS=linux
        export GOARCH=amd64
      '';

      installPhase = ''
        mkdir -p $out/bin
        cp bin/dividat-driver $out/bin/dividat-driver-linux-amd64
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

      inherit src;

      configurePhase = configurePhase + ''
        export GOOS=windows
        export GOARCH=amd64
      '';

      installPhase = ''
        mkdir -p $out/bin
        cp bin/dividat-driver $out/bin/dividat-driver-windows-amd64.exe
      '';

      nativeBuildInputs = with pkgs; [
        go_1_9
      ];

      buildInputs = with crossPkgs "x86_64-pc-mingw32"; [ 
        windows.mingw_w64_pthreads
      ];

    };

  }

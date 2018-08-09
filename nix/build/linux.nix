with (import ../nixpkgs.nix) { crossSystem = { config = "x86_64-unknown-linux-musl"; }; };

stdenv.mkDerivation {
  name = "dividat-driver";

  src = ./../../src;

  buildInputs = [
    (import ./../pcsclite {inherit stdenv fetchFromGitHub pkgconfig autoconf automake libtool flex python perl;}) pkgconfig
  ];
  
}

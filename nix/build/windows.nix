with (import ../nixpkgs.nix) { crossSystem = { config = "x86_64-pc-mingw32"; }; };

stdenv.mkDerivation {
  name = "dividat-driver";

  src = ./../../src;

  buildInputs = [
    windows.mingw_w64_pthreads
  ];
  
}

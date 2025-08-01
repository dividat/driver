{ pkgs }:
with pkgs;
mkShell
{
  buildInputs = [
    go
    gcc

    # node for tests
    nodejs

    # Required for building go dependencies
    autoconf
    automake
    libtool
    flex
    pkg-config
  ]
  ++ lib.optional stdenv.isLinux pcsclite
  ++ lib.optional stdenv.isDarwin pkgs.darwin.apple_sdk.frameworks.PCSC;
}

{ pkgs }:
with pkgs;
mkShell
{
  buildInputs = [
    go
    gcc

    # test dependencies
    nodejs_24
    # `install` causes warnings and is finally killed by OS on macOS CI runners with pnpm v11.4.0
    pnpm_10
    socat

    # Required for building go dependencies
    autoconf
    automake
    libtool
    flex
    pkg-config
  ]
  ++ lib.optional stdenv.isLinux pcsclite;
}

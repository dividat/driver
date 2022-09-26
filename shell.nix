with (import ./nix/nixpkgs.nix) {};

mkShell {

  buildInputs = [

    go_1_12
    gcc
    nix-prefetch-git

    # node for tests
    nodejs-10_x

    # for building releases
    openssl upx

    # for deployment to S3
    awscli

    # Required for building go dependencies
    autoconf automake libtool flex pkgconfig

  ] ++ lib.optional stdenv.isDarwin pkgs.darwin.apple_sdk.frameworks.PCSC # PCSC on Darwin
    ++ lib.optional stdenv.isLinux pcsclite;

  # GOPATH is set to a readonly directory
  # This seems to be fixed with nixpkgs 20.03
  # https://github.com/NixOS/nixpkgs/issues/90136
  shellHook = ''
    export GOPATH="$HOME/.go"
  '';

}

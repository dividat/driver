with (import ./nix/nixpkgs.nix) {};

buildGoPackage rec {
    name = "dividat-driver";
    goPackagePath = "dividat-driver";

    src = ./src/dividat-driver;

    goDeps = ./nix/deps.nix;

    buildInputs =
    [ 
        go_1_9
        dep
        # Git is a de facto dependency of dep
        git

        gcc

        nix-prefetch-git
        (import ./nix/deps2nix {inherit stdenv fetchFromGitHub buildGoPackage;})
        # node for tests
        nodejs-8_x

        # for building releases
        openssl upx

        # for signing windows releases
        (import ./nix/osslsigncode {inherit stdenv fetchurl openssl curl autoconf;})
        # for deployment to S3
        awscli

        # Required for building go dependencies
        autoconf automake libtool flex pkgconfig
      ]
      # PCSC on Darwin
      ++ lib.optional stdenv.isDarwin pkgs.darwin.apple_sdk.frameworks.PCSC
      ++ lib.optional stdenv.isLinux [ pcsclite ];

}

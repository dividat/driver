with (import ./nix/nixpkgs.nix) {};

let
  nodeDependencies = ((import ./nix/node {
    inherit pkgs;
    nodejs = nodejs-8_x;
  }).shell.override { src = ./nix/node/dummy; }).nodeDependencies;

  goPackagePath = "dividat-driver";

  goPath = import ./nix/make-gopath.nix {
    inherit pkgs lib;
    depsFile = ./nix/deps.nix;
  };

  crossbuild = import ./crossbuild.nix { inherit pkgs goPath goPackagePath; };

in

stdenv.mkDerivation {
    name = "dividat-driver";
    goPackagePath = "dividat-driver";

    src = ./src/dividat-driver;

    shellHook = ''
      echo ${crossbuild.windows}
      echo ${crossbuild.linux}
    '';

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
        nodeDependencies
				nodePackages.node2nix

        # for building releases
        openssl upx

        # for signing windows releases
        (import ./nix/osslsigncode {inherit stdenv fetchurl openssl curl autoconf;})
        # for deployment to S3
        awscli

        # Required for building go dependencies
        autoconf automake libtool flex pkgconfig

        pcsclite
      ]
      # PCSC on Darwin
      ++ lib.optional stdenv.isDarwin pkgs.darwin.apple_sdk.frameworks.PCSC
      ++ lib.optional stdenv.isLinux [ pcsclite ];

}

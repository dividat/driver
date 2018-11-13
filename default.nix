with (import ./nix/nixpkgs.nix) {};

let

  version = "2.2.0-dev";
  channel = "develop";
  releaseUrl = "https://dist.dividat.com/releases/driver2/";

  gitignore = import ./nix/gitignore.nix { inherit fetchFromGitHub lib; };

  src = gitignore.gitignoreSource ./.;

  nodeDependencies = ((import ./nix/node {
    inherit pkgs;
    nodejs = nodejs-8_x;
  }).shell.override { src = ./nix/node/dummy; }).nodeDependencies;

  goPath = import ./nix/make-gopath.nix {
    inherit pkgs lib;
    depsFile = ./nix/deps.nix;
  };

  crossbuild = import ./crossbuild.nix { inherit pkgs src version channel releaseUrl goPath; };

in

stdenv.mkDerivation rec {
    name = "dividat-driver-${version}";

    inherit src;

    # Enable test suite
    doCheck = true;
    checkTarget = "test";

    configurePhase = ''
      export GOPATH=${goPath}:`pwd`
      export VERSION=${version}
      export CHANNEL=${channel}
      export RELEASE_URL=${releaseUrl}
    '';

    installPhase = ''
      # Create the unsigned files for release
      mkdir -p $out/release-unsigned/${channel}/${version}
      cp ${crossbuild.linux}/bin/* $out/release-unsigned/${channel}/${version}
      cp ${crossbuild.windows}/bin/* $out/release-unsigned/${channel}/${version}
      echo ${version} > $out/release-unsigned/${channel}/latest

      # Copy binary for local system
      mkdir -p $out/bin
      cp bin/dividat-driver $out/bin
    '';

    shellHook = configurePhase;

    buildInputs =
    [ 
        go_1_9
        dep
        # Git is a de facto dependency of dep
        git

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
      ]
      # PCSC on Darwin
      ++ lib.optional stdenv.isDarwin pkgs.darwin.apple_sdk.frameworks.PCSC
      ++ lib.optional stdenv.isLinux pcsclite;

}

with (import ./nix/nixpkgs.nix) {};

let

  version = "2.2.0-dev";
  channel = "develop";
  releaseUrl = "https://dist.dividat.com/releases/driver2";
  releaseBucket = "s3://dist.dividat.ch/releases/driver2";

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

  # for signing windows releases
  osslsigncode = import ./nix/osslsigncode {inherit stdenv fetchurl openssl curl autoconf;};
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
    '';

    installPhase = ''
      # Create the unsigned files for release
      mkdir -p $out/release-unsigned/${channel}/${version}
      cp ${crossbuild.linux}/bin/dividat-driver-linux-amd64 \
        $out/release-unsigned/${channel}/${version}/dividat-driver-linux-amd64-${version}
      cp ${crossbuild.windows}/bin/dividat-driver-windows-amd64.exe \
        $out/release-unsigned/${channel}/${version}/dividat-driver-windows-amd64-${version}.exe
      echo ${version} > $out/release-unsigned/${channel}/latest

      # Copy binary for local system
      mkdir -p $out/bin
      cp bin/dividat-driver $out/bin

      # Deployment script
      substitute tools/deploy.sh $out/bin/deploy \
        --subst-var-by version "${version}" \
        --subst-var-by channel "${channel}" \
        --subst-var-by releaseUrl "${releaseUrl}" \
        --subst-var-by releaseBucket "${releaseBucket}" \
        --subst-var-by unsignedReleaseDir "$out/release-unsigned" \
        --subst-var-by upx "${upx}/bin/upx" \
        --subst-var-by openssl "${openssl}/bin/openssl" \
        --subst-var-by osslsigncode "${osslsigncode}/bin/osslsigncode" \
        --subst-var-by awscli "${awscli}/bin/aws" \
        --subst-var-by tree "${tree}/bin/tree" \
        --subst-var-by curl "${curl}/bin/curl"
      chmod +x $out/bin/deploy
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

        # Required for building go dependencies
        autoconf automake libtool flex pkgconfig
      ]
      # PCSC on Darwin
      ++ lib.optional stdenv.isDarwin pkgs.darwin.apple_sdk.frameworks.PCSC
      ++ lib.optional stdenv.isLinux pcsclite;

}

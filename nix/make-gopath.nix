{pkgs, lib, depsFile}:
with pkgs;
let
  dep2src = goDep:
    {
      inherit (goDep) goPackagePath;
      src = if goDep.fetch.type == "git" then
        fetchgit {
          inherit (goDep.fetch) url rev sha256;
        }
      else if goDep.fetch.type == "hg" then
        fetchhg {
          inherit (goDep.fetch) url rev sha256;
        }
      else if goDep.fetch.type == "bzr" then
        fetchbzr {
          inherit (goDep.fetch) url rev sha256;
        }
      else if goDep.fetch.type == "FromGitHub" then
        fetchFromGitHub {
          inherit (goDep.fetch) owner repo rev sha256;
        }
      else abort "Unrecognized package fetch type: ${goDep.fetch.type}";
    };

  godeps = map dep2src (import depsFile);

in
stdenvNoCC.mkDerivation {
  name = "gopath";
  buildCommand = ''
    mkdir $out
  ''+ lib.concatMapStrings ({ src, goPackagePath }: ''
    goPath=$TEMPDIR/goPath
    mkdir -p $goPath
    (cd $goPath; unpackFile "${src}")
    mkdir -p "$out/src/$(dirname "${goPackagePath}")"
    chmod -R u+w $goPath/*
    mv $goPath/* "$out/src/${goPackagePath}"
    rmdir $goPath
  '') godeps;
}



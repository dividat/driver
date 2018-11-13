{ fetchFromGitHub, lib }:
import (fetchFromGitHub {
  owner = "siers";
  repo = "nix-gitignore";
  rev = "5befe92dd12c8da05817b5eba5f6497abf81fcc3";
  sha256 = "13fr7grb40d5qxbfvf17pbb0pdy25sp7vmh7lv4rf17lvm537wn2";
}) { inherit lib; }


# This pins the version of nixpkgs
let
  _nixpkgs = import <nixpkgs> {};
in 
  import (_nixpkgs.fetchFromGitHub 
  { owner = "NixOS"
  ; repo = "nixpkgs"
  ; rev = "fa3ec9c8364eb2153d794b6a38cec2f8621d0afd"
  ; sha256 = "03c5q4mngbl8j87r7my53b261rmv1gpzp1vg1ql6s6gbjy9pbn92"; })


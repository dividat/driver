{ pkgs }:
with pkgs;
writeShellApplication {
  name = "mock-senso";
  runtimeInputs = [
    nodejs
    nodePackages.ts-node
  ] ++ lib.optionals stdenv.isLinux [ avahi ];
  text = ''
    if [ ! -f flake.nix ]; then
      echo "Run this from the root of the project."
      exit 1
    fi
    if [ ! -d mock-senso/node_modules ]; then
      echo "Installing mock-senso dependencies with npm..."
      cd mock-senso && npm install && cd ..
    fi
    ts-node mock-senso "$@"
  '';
}

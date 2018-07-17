# Dividat Driver

[![Build Status](https://travis-ci.org/dividat/driver.svg?branch=develop)](https://travis-ci.org/dividat/driver)

Dividat drivers and hardware test suites.

## Compatibility

Firefox, Safari and Edge not supported as they are not yet properly implementing _loopback as a trustworthy origin_, see:

-   Firefox (tracking): <https://bugzilla.mozilla.org/show_bug.cgi?id=1376309>
-   Edge: <https://developer.microsoft.com/en-us/microsoft-edge/platform/issues/11963735/>
-   Safari: <https://bugs.webkit.org/show_bug.cgi?id=171934>

## Development

### Prerequisites

[Nix](https://nixos.org/nix) is required for installing dependencies and providing a suitable development environment.

### Quick start

- Create a suitable environment: `nix-shell`
- Build the driver: `make`
- Run the driver: `./bin/dividat-driver`

### Tests

Run the test suite with: `make test`.

### Go packages

Go dependencies are provided by the [Go machinery](https://nixos.org/nixpkgs/manual/#sec-language-go) in Nix.

For local development you may use `dep` to install go dependencies: `cd src/dividat-driver && dep ensure`.

New Go dependencies can be added with `dep` (e.g. `dep ensure -add github.com/something/supercool`). The Nix specification of dependencies will recreated on subsequent builds (i.e. running `make`). Check in the updated `Gopkg.toml`, `Gopkg.lock` and `nix/deps.nix` files.

### Releasing

#### Building

**Currently releases can only be made from Linux.**

To create a release run: `make release`.

A default environment (defined in `default.nix`) provides all necessary dependencies for building on your native system (i.e. Linux or Darwin). Running `make` will create a binary that should run on your system (at least in the default environemnt).

Releases are built towards a more clearly specified target system (also statically linked). The target systems are defined in the [`nix/build`](nix/build) folder. Nix provides toolchains and dependencies for the target system in a sub environment. The build system (in the `make crossbuild` target) invokes these sub environments to build releases.

Existing release targets:

- Linux: statically linked with [musl](https://www.musl-libc.org/)
- Windows

### Deploying

To deploy a new release run: `make deploy`. This can only be done if you are on `master` or `develop` branch, have correctly tagged the revision and have AWS credentials set in your environment.

## Tools

### Data replayer

Logged data can be replayed for debugging purposes.

For default settings: `npm run replay`

To replay an other recording: `npm run replay -- rec/simple.dat`

To slow down the replay: `npm run replay -- -t 100`

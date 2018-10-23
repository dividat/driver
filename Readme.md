# Dividat Driver

[![Build status](https://badge.buildkite.com/6a69682e2acf50cec89f8c64935b8b591beda5635db479b92a.svg)](https://buildkite.com/dividat/driver)

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

### Node dependencies

Node dependencies are fetched and made available by Nix (using [`node2nix`](https://github.com/svanderburg/node2nix)).

You do not need to run `npm install`!

To add new packages use the usual `npm` commands and run `make node-dependencies`. This will rebuild the Nix declarations for setting up node dependencies.

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

## Installation

### Windows

This application can be run as a Windows service (<https://docs.microsoft.com/en-us/powershell/module/microsoft.powershell.management/new-service>).

A PowerShell script is provided to download and install the latest version as a Windows service. To run the script enter following command in an administrative PowerShell:

```
PS C:\ Set-ExecutionPolicy Bypass -Scope Process -Force; iex ((New-Object System.Net.WebClient).DownloadString('https://raw.githubusercontent.com/dividat/driver/master/install.ps1'))
```

Please have a look at the [script](install.ps1) before running it on your system.

## Tools

### Data recorder

Data from Senso can be recorded using the [`recorder`](src/dividat-driver/recorder). Start it with `make record > foo.dat`. The created recording can be used by the replayer.

### Data replayer

Recorded data can be replayed for debugging purposes.

For default settings: `npm run replay`

To replay an other recording: `npm run replay -- rec/simple.dat`

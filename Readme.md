# Dividat Driver

[![Build status](https://badge.buildkite.com/6a69682e2acf50cec89f8c64935b8b591beda5635db479b92a.svg)](https://buildkite.com/dividat/driver)

Dividat drivers and hardware test suites.

## Development

### Prerequisites

[Nix](https://nixos.org/nix) is required for installing dependencies and providing a suitable development environment.

### Quick start

- Enter the nix development shell: `nix develop`
- Build the driver: `make`
- Run the driver: `./bin/dividat-driver`

### Tests

Run the test suite with: `make test`.

### Go modules

To install a module, use `go get github.com/owner/repo`.

Documentation is available at https://golang.org/ref/mod.

### Formatting

Normalize formatting with: `make format`.

Code is formatted with `gofmt` and normalized formatting is required for CI to pass.

### Releasing

#### Building

**Currently releases can only be made from Linux.**

To create a release run: `make release`.

A default nix shell (defined in `nix/devShell.nix`) provides all necessary dependencies for building on your native system (i.e. Linux or Darwin). Running `make` will create a binary that should run on your system (at least in the default environemnt).

Releases are built as statically linked binaries for windows and linux using the cross compilation toolchain provided by nix. The toolchain is provided by nix shells defined in [crossBuild.nix](nix/crossBuild.nix). Building the binaries can be done by running `make crossbuild` from the default shell.

Existing official release targets:

- Linux: x86_64 (statically linked with [musl](https://www.musl-libc.org/))
- Windows: x86_64

There are also build targets for macOS binaries, but these are not hooked into `make crossbuild` as currently they only work on macOS.

To build the macOS binaries:

```sh
make crossbuild_mac
```

This will build binaries for Intel (amd64) and Silicon/M-series Macs (arm64).
Note: recent macOS versions prevent running unsigned arm64 binaries!

For non-technical-user convenience, we also provide macOS app bundles, which can
be built using:

```sh
make crossbuild_mac_bundles
```

All tagged versions of Driver also get automatically crossbuilt and published as
Github releases for convenience. See the [Release page for
details](https://github.com/dividat/driver/releases). **Binaries in Github
releases are not meant for official distribution/installations.**

### Deploying

To deploy a new release run: `make deploy`. This can only be done if you have correctly tagged the revision and have AWS credentials set in your environment.

## Installation

### macOS

The Driver can be manually run as macOS App for testing/demo purposes.

Latest versions are available in the [release page](https://github.com/dividat/driver/releases).

For recent M-series macOS computers (Apple Silicon), download the arm64 variant
called: `DividatDriver-arm64.app.zip`.

For older macOS computers (Intel-based), download the amd64 variant called
`DividatDriver-amd64.app.zip`.

They apps freake-signed (not notorized), so you have to [manually allow
execution in macOS Settings](https://support.apple.com/en-us/102445#openanyway).

### Windows

This application can be run as a Windows service (<https://docs.microsoft.com/en-us/powershell/module/microsoft.powershell.management/new-service>).

A PowerShell script is provided to download and install the latest version as a Windows service. Run it with the following command in a PowerShell.

**Note:** You need to run it as an administrator.

```
PS C:\ Set-ExecutionPolicy Bypass -Scope Process -Force; iex ((New-Object System.Net.WebClient).DownloadString('https://raw.githubusercontent.com/dividat/driver/main/install.ps1'))
```

Please have a look at the [script](install.ps1) before running it on your system.

## Compatibility

To be able to connect to the driver from within a web app delivered over HTTPS, browsers need to consider the loopback address as a trustworthy origin even when not using TLS. This is the case for most modern browsers, with the exception of Safari (https://bugs.webkit.org/show_bug.cgi?id=171934).

This application supports the [Private Network Access](https://wicg.github.io/private-network-access/) headers to help browsers decide which web apps may connect to it. The default list of [permissible origins](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Origin#syntax) consists of Dividat's app hosts. To restrict to a single origin or whitelist other origins, add one or more `--permissible-origin` parameters to the driver application.

## Tools

### Data recorder

#### Senso data

Data from Senso can be recorded using the [`recorder`](src/dividat-driver/recorder). Start it with `make record > foo.dat`. The created recording can be used by the replayer.

#### Senso Flex data

Like Senso data, but with `make record-flex`.

### Data replayer

Recorded data can be replayed for debugging purposes.

For default settings: `npm run replay`

To replay an other recording: `npm run replay -- rec/senso/simple.dat`

To change the replay speed: `npm run replay -- --speed=0.5 rec/senso/simple.dat`

To run without looping: `npm run replay -- --once`

#### Senso replay

The Senso replayer will appear as a Senso network device, so both driver and replayer should be running at the same time.

#### Senso Flex replay

The Senso Flex replayer (`npm run replay-flex`) supports the same parameters as the Senso replayer.

It mocks the driver with respect to the `/flex` WebSocket resource and the `/` metadata HTTP route, so the real driver can not be running at the same time.

You can control the mocked driver version via the `--driver-version` flag.

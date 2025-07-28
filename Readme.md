# Dividat Driver

Dividat drivers and hardware test suites.

## Development

### Prerequisites

[Nix](https://nixos.org/nix) is required for installing dependencies and providing a suitable development environment.

The default nix shell (defined in `nix/devShell.nix`) provides all necessary dependencies for building on your native system (i.e. Linux or Darwin).

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

### Release process

- Update Changelog with `$VERSION` and date
- Commit and tag the commit with: `git tag -a $VERSION -m $VERSION`
- Update changelog to add `[UNRELEASED]` heading, commit
- Push to `main`, including tag
- Verify new `$VERSION` becomes available in
  [Releases](https://github.com/dividat/driver/releases) (requires [Release workflow](https://github.com/dividat/driver/actions/workflows/release.yml) to complete).

### Static binaries

All tagged versions of Driver get automatically published as Github releases for
convenience. See the [Release page for
details](https://github.com/dividat/driver/releases).

Releases are built as statically linked binaries for Linux, Windows and macOS
using the cross compilation toolchain provided by nix (see
[crossBuild.nix](nix/crossBuild.nix)). 

Existing targets:

- Linux: x86_64 (statically linked with [musl](https://www.musl-libc.org/))
- Windows: x86_64
- macOS: x86_64 (Intel) and arm64 (M-series)

On Linux, it is only possible to crossbuild for Linux and Windows, using:

    make crossbuild

Crossbuilding for macOS requires a macOS host, using:

    make crossbuild_mac

This will build binaries for Intel (amd64) and Silicon/M-series Macs (arm64).
Note: recent macOS versions prevent running unsigned arm64 binaries!

For non-technical-user convenience, we also provide macOS app bundles, which can
be built using:

```sh
make crossbuild_mac_bundles
```

Note: binaries in Github releases are not meant for official installations,
since they are not signed. E.g. [PlayOS](https://github.com/dividat/playos)
builds the Driver from source.

## Installation

Driver can be run as standalone application for testing/demo purposes.

Latest versions are available in the [release page](https://github.com/dividat/driver/releases).

### Windows

Download the `dividat-driver-windows-amd64.exe` file from the release page and
run it. You might need to grant access to the local network.

### macOS

Both plain binaries and app bundles are provided for macOS. For convenience, we
recommend using the app bundles.

For recent M-series macOS computers (Apple Silicon), download the arm64 variant
called: `DividatDriver-arm64.app.zip`.

For older macOS computers (Intel-based), download the amd64 variant called
`DividatDriver-amd64.app.zip`.

Because they are not intended for regular usage, these apps are only fake-signed (not notarized),
so you have to [manually allow execution in macOS Settings](https://support.apple.com/en-us/102445#openanyway).

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

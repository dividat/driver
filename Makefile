### Release configuration #################################
# Path to folder in S3 (without slash at end)
BUCKET = s3://dist.dividat.ch/releases/driver2

# where the BUCKET folder is accessible for getting updates (needs to end with a slash)
RELEASE_URL = https://dist.dividat.com/releases/driver2/


### Basic setup ###########################################
# Set GOPATH to repository path
CWD = $(dir $(realpath $(firstword $(MAKEFILE_LIST))))
GOPATH ?= $(CWD)

# Main source to build
SRC = ./src/dividat-driver/main.go

# Default location where built binary will be placed
OUT ?= bin/dividat-driver

# Only channel is main now
CHANNEL := main

VERSION ?= $(shell git describe --always HEAD)

CHECKSUM_SIGNING_CERT ?= ./keys/checksumsign.private.pem


### Simple debug build ####################################
.PHONY: build
build:
		@./build.sh -i $(SRC) -o $(OUT) -v $(VERSION) -t debug


### Test suite ############################################
.PHONY: test
test: build
	npm install
	npm test


### Formatting ############################################
.PHONY: format
format:
	gofmt -w src/


### Helper to quickly run the driver
.PHONY: run
run: build
	$(OUT)

### Helper to start the recorder
.PHONY: record
record:
	@go run src/dividat-driver/recorder/main.go ws://localhost:8382/senso

### Helper to start the recorder for Flex
.PHONY: record-flex
record-flex:
	@go run src/dividat-driver/recorder/main.go ws://localhost:8382/flex

### Cross compilation #####################################
LINUX_BIN = bin/dividat-driver-linux-amd64
.PHONY: $(LINUX_BIN)
$(LINUX_BIN):
	nix develop '.#crossBuild.x86_64-linux' --command bash -c "VERBOSE=1 ./build.sh -i $(SRC) -o $(LINUX_BIN) -v $(VERSION) "

WINDOWS_BIN = bin/dividat-driver-windows-amd64.exe
.PHONY: $(WINDOWS_BIN)
$(WINDOWS_BIN):
	nix develop '.#crossBuild.x86_64-windows' --command bash -c "VERBOSE=1 ./build.sh -i $(SRC) -o $(WINDOWS_BIN) -v $(VERSION)"

crossbuild: $(LINUX_BIN) $(WINDOWS_BIN)

### macOS cross-compilation and app bundle #############

MACOS_ARM_BIN = bin/dividat-driver-darwin-arm64
.PHONY: $(MACOS_ARM_BIN)
$(MACOS_ARM_BIN):
	nix develop '.#crossBuild.darwin.aarch64' --command bash -c "VERBOSE=1 ./build.sh -i $(SRC) -o $@ -v $(VERSION)"

MACOS_X86_BIN = bin/dividat-driver-darwin-amd64
.PHONY: $(MACOS_X86_BIN)
$(MACOS_X86_BIN):
	nix develop '.#crossBuild.darwin.x86_64' --command bash -c "VERBOSE=1 ./build.sh -i $(SRC) -o $@ -v $(VERSION)"

MACOS_ARM_APP_BUNDLE = bin/DividatDriver-arm64.app
MACOS_ARM_APP_BUNDLE_ZIP = $(MACOS_ARM_APP_BUNDLE).zip
MACOS_X86_APP_BUNDLE = bin/DividatDriver-amd64.app
MACOS_X86_APP_BUNDLE_ZIP = $(MACOS_X86_APP_BUNDLE).zip

bin/DividatDriver-%.app: bin/dividat-driver-darwin-% macos/Info.plist macos/launcher
	mkdir -p $@/Contents/MacOS
	cp $< $@/Contents/MacOS/driver
	chmod +x $@/Contents/MacOS/driver
	cp macos/Info.plist $@/Contents/Info.plist
	cp macos/launcher $@/Contents/MacOS/launcher
	chmod +x $@/Contents/MacOS/launcher
	sudo codesign --deep --force --sign "-" $@

bin/DividatDriver-%.app.zip: bin/DividatDriver-%.app
	zip -r -y $@ $<

.PHONY: crossbuild_mac
crossbuild_mac: $(MACOS_X86_BIN) $(MACOS_ARM_BIN)

.PHONY: crossbuild_mac_bundles
crossbuild_mac_bundles: $(MACOS_X86_APP_BUNDLE_ZIP) $(MACOS_ARM_APP_BUNDLE_ZIP)

clean:
	rm -rf bin/
	go clean src/dividat-driver/main.go

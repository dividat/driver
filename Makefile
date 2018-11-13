### Release configuration #################################
# Path to folder in S3 (without slash at end)
BUCKET = s3://dist.dividat.ch/releases/driver2

# where the BUCKET folder is accessible for getting updates (needs to end with a slash)
RELEASE_URL ?= https://dist.dividat.com/releases/driver2/


### Basic setup ###########################################
# Set GOPATH to repository path
CWD = $(dir $(realpath $(firstword $(MAKEFILE_LIST))))
GOPATH ?= $(CWD)

# Main source to build
SRC = ./src/dividat-driver/main.go

# Get version from git
VERSION ?= $(shell git describe --always HEAD)

# set the channel name to the branch name
CHANNEL ?= $(shell git rev-parse --abbrev-ref HEAD)

# Enable static linking
ifdef STATIC_BUILD
	STATIC_LINKING_LDFLAGS := -linkmode external -extldflags \"-static\"
endif

GO_LDFLAGS = -ldflags "$(STATIC_LINKING_LDFLAGS) -X dividat-driver/server.channel=$(CHANNEL) -X dividat-driver/server.version=$(VERSION) -X dividat-driver/update.releaseUrl=$(RELEASE_URL)"

CODE_SIGNING_CERT ?= ./keys/codesign.p12
CHECKSUM_SIGNING_CERT ?= ./keys/checksumsign.private.pem


### Simple build ##########################################
.PHONY: bin/dividat-driver
bin/dividat-driver:
	mkdir -p bin
	go build $(GO_LDFLAGS) -o $@ $(SRC)


### Test suite ############################################
.PHONY: test
test: bin/dividat-driver
	npm test


### Helper to quickly run the driver
.PHONY: run
run:
	go run $(GO_LDFLAGS) $(SRC)

### Helper to start the recorder
.PHONY: recorder
recorder:
	@go run src/dividat-driver/recorder/main.go


### Release ###############################################

RELEASE_DIR = release/$(CHANNEL)/$(VERSION)

# Helper to create signature
define write-signature
	openssl dgst -sha256 -sign $(CHECKSUM_SIGNING_CERT) $(1) | openssl base64 -A -out $(1).sig
	# make sure signature file exists and is non-zero
	test -s $(1).sig
endef

# write the pointer to the latest release
LATEST = release/$(CHANNEL)/latest
.PHONY: $(LATEST)
$(LATEST):
	mkdir -p `dirname $(LATEST)`
	echo $(VERSION) > $@
	$(call write-signature,$@)

LINUX_RELEASE = $(RELEASE_DIR)/$(notdir $(LINUX_BIN))-$(VERSION)
$(LINUX_RELEASE): $(LINUX_BIN)
	mkdir -p $(RELEASE_DIR)
	cp $(LINUX_BIN) $(LINUX_RELEASE)
	upx $(LINUX_RELEASE)
	$(call write-signature,$(LINUX_RELEASE))

#DARWIN_RELEASE = $(RELEASE_DIR)/$(notdir $(DARWIN_BIN))-$(VERSION)
#$(DARWIN_RELEASE): $(DARWIN_BIN)
	#cp bin/$(DARWIN_BIN) $(DARWIN_RELEASE)
	#upx $(DARWIN_RELEASE)
	#$ (call write-signature,$(DARWIN_RELEASE))

WINDOWS_RELEASE = $(RELEASE_DIR)/dividat-driver-windows-amd64-$(VERSION).exe
$(WINDOWS_RELEASE): $(WINDOWS_BIN)
	cp $(WINDOWS_BIN) $(WINDOWS_RELEASE)
	upx $(WINDOWS_BIN)
	osslsigncode sign \
		-pkcs12 $(CODE_SIGNING_CERT) \
		-h sha1 \
		-n "Dividat Driver" \
		-i "https://www.dividat.com/" \
		-t http://timestamp.verisign.com/scripts/timstamp.dll \
		-in $(WINDOWS_BIN) \
		-out $(WINDOWS_RELEASE)
	$(call write-signature,$(WINDOWS_RELEASE))

# sign and copy binaries to release folders
.PHONY: release
release: $(LINUX_RELEASE) $(WINDOWS_RELEASE) release/$(CHANNEL)/latest


### Deploy ################################################

SEMVER_REGEX = ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(\-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$

deploy: release
	# Check if on right channel
	[[ $(CHANNEL) = "master" || $(CHANNEL) = "develop" ]]

	# Check if version is in semver format
	[[ $(VERSION) =~ $(SEMVER_REGEX) ]]

	# Print information about channel heads and confirm
	@echo "Channel 'master' is at:"
	@echo "  $(shell git show --oneline --decorate --quiet $$(curl -s "$(RELEASE_URL)master/latest" | tr -d '\n') | tail -1)"
	@echo "Channel 'develop' is at:"
	@echo "  $(shell git show --oneline --decorate --quiet $$(curl -s "$(RELEASE_URL)develop/latest" | tr -d '\n') | tail -1)"
	@echo
	@echo "About to deploy $(VERSION) to '$(CHANNEL)'. Proceed? [y/N]" && read ans && [ $${ans:-N} == y ]

	aws s3 cp $(RELEASE_DIR) $(BUCKET)/$(CHANNEL)/$(VERSION)/ --recursive \
		--acl public-read \
		--cache-control max-age=0
	aws s3 cp $(LATEST) $(BUCKET)/$(CHANNEL)/latest \
		--acl public-read \
		--cache-control max-age=0
	aws s3 cp $(LATEST).sig $(BUCKET)/$(CHANNEL)/latest.sig \
		--acl public-read \
		--cache-control max-age=0


### Dependencies and cleanup ##############################
nix/deps.nix: src/dividat-driver/Gopkg.toml
	dep2nix -i src/dividat-driver/Gopkg.lock -o nix/deps.nix

.PHONY: node-dependencies
node-dependencies: package.json
	cp package.json nix/node/dummy/package.json
	node2nix -8 \
		--development \
		--composition nix/node/default.nix \
    --node-env nix/node/env.nix \
    --output nix/node/packages.nix

clean:
	rm -rf release/
	rm -rf bin/
	rm -rf src/dividat-driver/vendor/
	go clean

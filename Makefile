### Basic setup ###########################################
# Main source to build
SRC = ./src/dividat-driver/main.go

# Get version from git if not already set
VERSION ?= $(shell git describe --always HEAD)

# set the channel name to the branch name if not already set
CHANNEL ?= $(shell git rev-parse --abbrev-ref HEAD)

# Enable static linking
ifdef STATIC_BUILD
	STATIC_LINKING_LDFLAGS := -linkmode external -extldflags \"-static\"
endif

GO_LDFLAGS = -ldflags "$(STATIC_LINKING_LDFLAGS) -X dividat-driver/server.channel=$(CHANNEL) -X dividat-driver/server.version=$(VERSION) -X dividat-driver/update.releaseUrl=$(RELEASE_URL)"


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


### Dependencies ##########################################
.PHONY: update-dependencies
update-dependencies: nix/deps.nix node-dependencies

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

### Clean up
clean:
	rm -rf bin/
	rm -rf src/dividat-driver/vendor/
	go clean

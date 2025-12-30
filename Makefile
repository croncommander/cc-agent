BINARY_NAME=cc-agent
BUILD_DIR=bin
LDFLAGS=-ldflags "-s -w"

PLATFORMS=linux/amd64 linux/arm64 linux/386 darwin/amd64 darwin/arm64 freebsd/amd64 freebsd/arm64 freebsd/386

.PHONY: all clean build $(PLATFORMS)

all: clean build

build: $(PLATFORMS)

$(PLATFORMS):
	@echo "Building for $@"
	@mkdir -p $(BUILD_DIR)
	@GOOS=$(word 1, $(subst /, ,$@)) GOARCH=$(word 2, $(subst /, ,$@)) CGO_ENABLED=0 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-$(word 1, $(subst /, ,$@))-$(word 2, $(subst /, ,$@)) .

VERSION ?= 1.1.0
RELEASE_DIR = releases/v$(VERSION)

clean:
	@echo "Cleaning executeables..."
	@rm -rf $(BUILD_DIR)

publish: build
	@echo "Creating release $(VERSION)..."
	@mkdir -p $(RELEASE_DIR)
	@cp -r $(BUILD_DIR)/* $(RELEASE_DIR)/
	@cp install.sh $(RELEASE_DIR)/
	@echo "Release created at $(RELEASE_DIR)"
	
	@echo "Syncing to ../build for local development..."
	@mkdir -p ../build
	@cp -r $(BUILD_DIR)/* ../build/
	@cp install.sh ../build/
	@echo "Build sync complete."

BINARY_NAME=cc-agent
BUILD_DIR=bin
LOCAL_BUILD_DIR=../build
VERSION := $(shell cat VERSION 2>/dev/null || echo "0.0.0")
VERSION_DASHED := $(subst .,-,$(VERSION))
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION)"

PLATFORMS=linux/amd64 linux/arm64 linux/386 darwin/amd64 darwin/arm64 freebsd/amd64 freebsd/arm64 freebsd/386

.PHONY: all clean build local-linux-amd64 publish $(PLATFORMS)

all: clean build

build: $(PLATFORMS)

$(PLATFORMS):
	@echo "Building $(BINARY_NAME)-$(VERSION_DASHED)-$(word 1, $(subst /, ,$@))-$(word 2, $(subst /, ,$@))"
	@mkdir -p $(BUILD_DIR)
	@GOOS=$(word 1, $(subst /, ,$@)) GOARCH=$(word 2, $(subst /, ,$@)) CGO_ENABLED=0 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION_DASHED)-$(word 1, $(subst /, ,$@))-$(word 2, $(subst /, ,$@)) .
	@cd $(BUILD_DIR) && sha256sum $(BINARY_NAME)-$(VERSION_DASHED)-$(word 1, $(subst /, ,$@))-$(word 2, $(subst /, ,$@)) > $(BINARY_NAME)-$(VERSION_DASHED)-$(word 1, $(subst /, ,$@))-$(word 2, $(subst /, ,$@)).sha256

clean:
	@echo "Cleaning executables..."
	@rm -rf $(BUILD_DIR)

local-linux-amd64: linux/amd64
	@mkdir -p $(LOCAL_BUILD_DIR)
	@cp $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION_DASHED)-linux-amd64 $(LOCAL_BUILD_DIR)/$(BINARY_NAME)-linux-amd64
	@cd $(LOCAL_BUILD_DIR) && sha256sum $(BINARY_NAME)-linux-amd64 > $(BINARY_NAME)-linux-amd64.sha256
	@echo "Synced $(BINARY_NAME) v$(VERSION) to $(LOCAL_BUILD_DIR)/$(BINARY_NAME)-linux-amd64"

publish: build
	@echo "Publishing release v$(VERSION)..."
	@mkdir -p releases
	@cp $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION_DASHED)-* releases/
	@echo "Release binaries copied to releases/"
	@echo ""
	@echo "Syncing to ../build for local development..."
	@mkdir -p ../build
	@cp $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION_DASHED)-* ../build/
	@echo "Build sync complete."

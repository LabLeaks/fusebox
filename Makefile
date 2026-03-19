BINARY     := fusebox
CONFIG     := $(HOME)/.config/fusebox/config.yaml
SERVER     ?= $(shell awk '/^server:/{found=1} found && /^ *host:/{print $$2; exit}' $(CONFIG) 2>/dev/null)
SERVER_USER?= $(shell awk '/^server:/{found=1} found && /^ *user:/{print $$2; exit}' $(CONFIG) 2>/dev/null)
REMOTE_BIN := /home/$(SERVER_USER)/bin

PREFIX     := $(HOME)/.local
DESTDIR    :=

.PHONY: build build-server install uninstall deploy setup clean test test-server release embed-server rootfs

## build: compile the fusebox binary
build:
	go build -o $(BINARY) .

## build-server: cross-compile fusebox binary for linux/arm64
build-server:
	GOOS=linux GOARCH=arm64 go build -o fusebox-server .

## install: build and install the client binary locally
install: build
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 755 $(BINARY) $(DESTDIR)$(PREFIX)/bin/$(BINARY)

## uninstall: remove the client binary
uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/$(BINARY)

## deploy: build + install client locally, cross-compile server binary, push to server
deploy: install build-server
	@test -n "$(SERVER)" || (echo "Error: SERVER not set. Use: make deploy SERVER=host SERVER_USER=user" && exit 1)
	@test -n "$(SERVER_USER)" || (echo "Error: SERVER_USER not set. Use: make deploy SERVER=host SERVER_USER=user" && exit 1)
	@# Ensure local config exists with server details
	@mkdir -p $(dir $(CONFIG))
	@if [ ! -f $(CONFIG) ]; then \
		printf 'server:\n  host: $(SERVER)\n  user: $(SERVER_USER)\n\nbrowse_roots:\n  - ~/work\n' > $(CONFIG); \
		echo "Created $(CONFIG)"; \
	elif ! grep -q '^ *host:' $(CONFIG); then \
		printf 'server:\n  host: $(SERVER)\n  user: $(SERVER_USER)\n\nbrowse_roots:\n  - ~/work\n' > $(CONFIG); \
		echo "Updated $(CONFIG)"; \
	fi
	ssh $(SERVER_USER)@$(SERVER) 'mkdir -p $(REMOTE_BIN) $$HOME/.config/fusebox'
	scp fusebox-server $(SERVER_USER)@$(SERVER):$(REMOTE_BIN)/fusebox
	ssh $(SERVER_USER)@$(SERVER) 'ln -sf $(REMOTE_BIN)/fusebox $(REMOTE_BIN)/fusebox-helper'
	@# Generate roots.conf from config.yaml browse_roots (skip if no browse_roots)
	@ROOTS=$$(grep -A 50 'browse_roots:' $(CONFIG) 2>/dev/null | tail -n +2 | grep '^ *-' | sed 's/^ *- *//' | sed "s|^~|/home/$(SERVER_USER)|"); \
	if [ -n "$$ROOTS" ]; then \
		echo "$$ROOTS" | ssh $(SERVER_USER)@$(SERVER) 'cat > $$HOME/.config/fusebox/roots.conf'; \
	fi
	ssh $(SERVER_USER)@$(SERVER) '$(REMOTE_BIN)/fusebox install-hooks'
	ssh $(SERVER_USER)@$(SERVER) '$(REMOTE_BIN)/fusebox fix-mouse'
	@echo ""
	@echo "Deployed: fusebox binary + roots.conf."

## setup: alias for deploy
setup: deploy

## test: run all Go tests
test:
	go test ./... -count=1

## test-server: run server integration tests (needs tmux)
test-server:
	go test -tags integration -count=1 -v ./internal/server/

## test-e2e: run end-to-end tests (needs docker)
test-e2e:
	test/e2e/run.sh

## release: build with embedded server binaries
release: embed-server
	go build -tags embed_server -o $(BINARY) .

## embed-server: cross-compile linux server binaries for embedding
embed-server:
	GOOS=linux GOARCH=arm64 go build -o internal/embed/fusebox-linux-arm64 .
	GOOS=linux GOARCH=amd64 go build -o internal/embed/fusebox-linux-amd64 .

## rootfs: build rootfs tarballs (requires Docker with buildx + QEMU)
rootfs:
	chmod +x rootfs/build.sh
	rootfs/build.sh rootfs/

## clean: remove build artifacts
clean:
	rm -f $(BINARY) fusebox-server internal/embed/fusebox-linux-arm64 internal/embed/fusebox-linux-amd64
	rm -f rootfs/fusebox-rootfs-*.tar.gz

BINARY     := work
CONFIG     := $(HOME)/.config/work-cli/config.yaml
SERVER     ?= $(shell awk '/^server:/{found=1} found && /^ *host:/{print $$2; exit}' $(CONFIG) 2>/dev/null)
SERVER_USER?= $(shell awk '/^server:/{found=1} found && /^ *user:/{print $$2; exit}' $(CONFIG) 2>/dev/null)
REMOTE_BIN := /home/$(SERVER_USER)/bin

PREFIX     := $(HOME)/.local
DESTDIR    :=

.PHONY: build build-server install uninstall deploy setup clean test test-server release embed-server

## build: compile the work binary
build:
	go build -o $(BINARY) .

## build-server: cross-compile work binary for linux/arm64
build-server:
	GOOS=linux GOARCH=arm64 go build -o work-server .

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
	ssh $(SERVER_USER)@$(SERVER) 'mkdir -p $(REMOTE_BIN) $$HOME/.config/work-cli'
	scp work-server $(SERVER_USER)@$(SERVER):$(REMOTE_BIN)/work
	ssh $(SERVER_USER)@$(SERVER) 'ln -sf $(REMOTE_BIN)/work $(REMOTE_BIN)/work-helper'
	@# Generate roots.conf from config.yaml browse_roots (skip if no browse_roots)
	@ROOTS=$$(grep -A 50 'browse_roots:' $(CONFIG) 2>/dev/null | tail -n +2 | grep '^ *-' | sed 's/^ *- *//' | sed "s|^~|/home/$(SERVER_USER)|"); \
	if [ -n "$$ROOTS" ]; then \
		echo "$$ROOTS" | ssh $(SERVER_USER)@$(SERVER) 'cat > $$HOME/.config/work-cli/roots.conf'; \
	fi
	ssh $(SERVER_USER)@$(SERVER) '$(REMOTE_BIN)/work install-hooks'
	ssh $(SERVER_USER)@$(SERVER) '$(REMOTE_BIN)/work fix-mouse'
	@echo ""
	@echo "Deployed: work binary + roots.conf."

## setup: alias for deploy
setup: deploy

## test: run all Go tests
test:
	go test ./... -count=1

## test-server: run server integration tests (needs tmux)
test-server:
	go test -tags integration -count=1 -v ./internal/server/

## release: build with embedded server binaries
release: embed-server
	go build -tags embed_server -o $(BINARY) .

## embed-server: cross-compile linux server binaries for embedding
embed-server:
	GOOS=linux GOARCH=arm64 go build -o internal/embed/work-linux-arm64 .
	GOOS=linux GOARCH=amd64 go build -o internal/embed/work-linux-amd64 .

## clean: remove build artifacts
clean:
	rm -f $(BINARY) work-server internal/embed/work-linux-arm64 internal/embed/work-linux-amd64

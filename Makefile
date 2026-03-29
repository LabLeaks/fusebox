BINARY   := fusebox
BUILD    := ./cmd/fusebox
GOFLAGS  := CGO_ENABLED=0

.PHONY: all build build-remote install clean test

all: build build-remote

build:
	$(GOFLAGS) go build -o $(BINARY) $(BUILD)

build-remote:
	GOOS=linux GOARCH=amd64 $(GOFLAGS) go build -o $(BINARY)-linux-amd64 $(BUILD)

install:
	$(GOFLAGS) go install $(BUILD)

clean:
	rm -f $(BINARY) $(BINARY)-linux-amd64

test:
	go test ./...

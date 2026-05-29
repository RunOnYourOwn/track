.PHONY: build check test vet install clean deploy

# Stamp the binary with the version (git tag, else commit, else "dev") so
# `track --version`, the MCP serverInfo, and the web UI all report it.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X github.com/RunOnYourOwn/track/internal/version.Version=$(VERSION)"

check: fmt vet test build

fmt:
	@test -z "$$(gofmt -l .)" || { echo "Unformatted files:"; gofmt -l .; exit 1; }

vet:
	go vet ./...

test:
	go test ./... -count=1

build:
	go build $(LDFLAGS) -o track .

# `make install` uses `go install` (a same-volume rename into GOBIN), not `cp` to
# ~/bin: on Apple Silicon macOS Sequoia, copying the signed Mach-O re-touches its
# xattrs (com.apple.provenance) and invalidates the ad-hoc code signature, so the
# copy gets SIGKILLed on launch. GOBIN pins the install dir to ~/bin as before.
install: check
	GOBIN=$(HOME)/bin go install $(LDFLAGS) .

deploy: check install
	@echo "Deployed to ~/bin/track"

clean:
	rm -f track

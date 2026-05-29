.PHONY: build check test vet install clean deploy

check: fmt vet test build

fmt:
	@test -z "$$(gofmt -l .)" || { echo "Unformatted files:"; gofmt -l .; exit 1; }

vet:
	go vet ./...

test:
	go test ./... -count=1

build:
	go build -o track .

install: check
	cp track ~/bin/track

deploy: check install
	@echo "Deployed to ~/bin/track"

clean:
	rm -f track

.PHONY: build check test vet install clean deploy

check: vet test build

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

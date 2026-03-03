.PHONY: build test install

build:
	go build -trimpath -o devshare ./cmd/devshare

test:
	go test ./...

install: build
	install -Dm755 devshare "$(HOME)/.local/bin/devshare"

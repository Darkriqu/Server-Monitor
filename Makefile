BINARY := server-monitor
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test lint fmt frontend clean
build: frontend
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/server-monitor

frontend:
	@mkdir -p web/dist
	@cp web/src/app.ts web/dist/app.js

test:
	go test ./...

lint:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go')

clean:
	rm -rf bin

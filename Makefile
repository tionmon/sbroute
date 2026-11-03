TAGS = with_quic,with_utls
VERSION ?= $(patsubst v%,%,$(shell git describe --tags --always --dirty 2>/dev/null || echo dev))
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
LDFLAGS = -s -w -X github.com/tionmon/sbroute/internal/model.Version=$(VERSION) -X github.com/tionmon/sbroute/internal/model.Commit=$(COMMIT)
.PHONY: build web test vet lint bench linux
web:
	cd web && npm ci && npm run build
build: web
	CGO_ENABLED=0 go build -tags $(TAGS) -trimpath -ldflags '$(LDFLAGS)' -o bin/sbroute ./cmd/sbroute
test:
	go test -race -tags $(TAGS) ./... -count=1 -timeout=180s
	cd web && npm test
vet:
	go vet -tags $(TAGS) ./...
lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.2 run ./...
bench:
	go test -tags $(TAGS) ./internal/core -run '^$$' -bench . -benchmem -count=3
linux: web
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags $(TAGS) -trimpath -ldflags '$(LDFLAGS)' -o bin/sbroute-linux-amd64 ./cmd/sbroute
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags $(TAGS) -trimpath -ldflags '$(LDFLAGS)' -o bin/sbroute-linux-arm64 ./cmd/sbroute

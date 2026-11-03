TAGS = with_quic,with_utls
.PHONY: build web test vet bench linux
web:
	cd web && npm ci && npm run build
build: web
	CGO_ENABLED=0 go build -tags $(TAGS) -trimpath -ldflags '-s -w' -o bin/sbroute ./cmd/sbroute
test:
	go test -race -tags $(TAGS) ./... -count=1 -timeout=180s
	cd web && npm test
vet:
	go vet -tags $(TAGS) ./...
bench:
	go test -tags $(TAGS) ./internal/core -run '^$$' -bench . -benchmem -count=3
linux: web
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags $(TAGS) -trimpath -ldflags '-s -w' -o bin/sbroute-linux-amd64 ./cmd/sbroute
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags $(TAGS) -trimpath -ldflags '-s -w' -o bin/sbroute-linux-arm64 ./cmd/sbroute

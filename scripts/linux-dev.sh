#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p .tools bin
if [[ ! -x .tools/go/bin/go ]]; then
  version=go1.26.4
  archive="$version.linux-amd64.tar.gz"
  curl -fL --retry 3 "https://go.dev/dl/$archive" -o ".tools/$archive"
  expected=$(curl -fsSL 'https://go.dev/dl/?mode=json&include=all' | python3 -c "import json,sys; print(next(f['sha256'] for r in json.load(sys.stdin) if r['version']=='$version' for f in r['files'] if f['filename']=='$archive'))")
  echo "$expected  .tools/$archive" | sha256sum --check --status
  tar -xzf ".tools/$archive" -C .tools
fi
export PATH="$PWD/.tools/go/bin:$PATH"
export GOMAXPROCS=2
export GOFLAGS='-p=2'
go version
case "${1:-test}" in
  test) go test -race -tags with_quic,with_utls ./... -count=1 -timeout=180s ;;
  build) CGO_ENABLED=0 go build -tags with_quic,with_utls -trimpath -ldflags '-s -w' -o bin/sbroute ./cmd/sbroute ;;
  bench) go test -tags with_quic,with_utls ./internal/core -run '^$' -bench . -benchmem -count=3 ;;
  *) echo 'usage: scripts/linux-dev.sh [test|build|bench]' >&2; exit 2 ;;
esac

#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
state=$(mktemp -d /tmp/sbroute-e2e.XXXXXX)
printf '%s' 'local-e2e-only-12345' | bin/sbroute admin --data-dir "$state" --password-stdin
exec bin/sbroute serve --data-dir "$state" --listen 127.0.0.1:18080

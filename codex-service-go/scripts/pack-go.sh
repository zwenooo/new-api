#!/usr/bin/env bash
set -euo pipefail
OUTDIR=${1:-dist}
DIR="$(cd "$(dirname "$0")"/.. && pwd)"
cd "$DIR"

mkdir -p "$OUTDIR"
echo "Building codex-service-go (server) ..."
GOOS=$(go env GOOS) GOARCH=$(go env GOARCH) go build -o "$OUTDIR/codex-service-go" ./cmd/server

echo "Building sse-replay tool ..."
GOOS=$(go env GOOS) GOARCH=$(go env GOARCH) go build -o "$OUTDIR/sse-replay" ./cmd/sse-replay

cp -f .env.example "$OUTDIR/.env.example" || true
cat > "$OUTDIR/start.sh" <<'EOS'
#!/usr/bin/env bash
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
"$DIR/codex-service-go"
EOS
chmod +x "$OUTDIR/start.sh"
echo "Packed to $OUTDIR"


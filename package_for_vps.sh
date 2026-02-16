#!/usr/bin/env bash
set -euo pipefail

# Config
APP_NAME="app"
OUTPUT_DIR="vps-bundle"
DIST_DIR="web-app/dist"

echo "💎 Starting Optimized Local Build for VPS..."

# 1. Cleanup
echo "🧹 Cleaning up..."
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

# 2. Build Frontend
echo "🎨 Building Frontend..."
cd web-app
npm install --silent
npm run build
cd ..

# 3. Build Go Binary (Cross-Compile for Linux AMD64)
echo "🐹 Building Go Binary (scripts/build_linux_amd64)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=$(git describe --tags --always) -X main.Commit=$(git rev-parse --short HEAD) -X main.BuildDate=$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
    -o "$OUTPUT_DIR/app" ./cmd/app

# 4. Copy Assets
echo "📦 Copying assets..."
cp -r "$DIST_DIR" "$OUTPUT_DIR/web-app-dist"
cp -r translations "$OUTPUT_DIR/translations"
cp -r db "$OUTPUT_DIR/db"
cp Dockerfile.prebuilt "$OUTPUT_DIR/Dockerfile"
cp docker-compose.prebuilt.yml "$OUTPUT_DIR/docker-compose.yml"
cp .env "$OUTPUT_DIR/.env" 2>/dev/null || echo "⚠️  No .env found, please create one in the output dir!"

# 5. Instructions
echo "✅ Build Complete!"
echo "--------------------------------------------------------"
echo "Deploy Instructions:"
echo "1. Upload the '$OUTPUT_DIR' folder to your VPS: scp -r $OUTPUT_DIR user@host:~/"
echo "2. SSH into VPS: ssh user@host"
echo "3. Go to folder: cd ~/vps-bundle"
echo "4. Run: docker compose up -d --build"
echo "--------------------------------------------------------"
cp Caddyfile "$OUTPUT_DIR/Caddyfile"

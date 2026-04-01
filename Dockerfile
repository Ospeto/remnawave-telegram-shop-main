FROM --platform=$BUILDPLATFORM golang:1.25.3-alpine AS modules
WORKDIR /modules
COPY go.mod go.sum ./
RUN go mod download

# ── Build frontend ──────────────────────────────────────────
FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend
WORKDIR /frontend
COPY web-app/package.json web-app/package-lock.json ./
RUN npm ci --silent
COPY web-app/ ./
RUN npm run build

# ── Build Go binary ─────────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.25.3-alpine AS builder
WORKDIR /app

COPY --from=modules /go/pkg /go/pkg

COPY . .

RUN apk update && apk add --no-cache ca-certificates tzdata
RUN update-ca-certificates

ARG TARGETOS
ARG TARGETARCH
ARG VERSION
ARG COMMIT=none

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s -X main.Version=${VERSION:-dev} -X main.Commit=${COMMIT:-none} -X main.BuildDate=$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
    -o /bin/app ./cmd/app

# ── Final image ─────────────────────────────────────────────
FROM alpine:3.22

ARG VERSION
ARG COMMIT

LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${COMMIT}"
LABEL org.opencontainers.image.source="https://github.com/${GITHUB_REPOSITORY}"
LABEL org.opencontainers.image.description="Remnawave Telegram Shop Bot"
LABEL org.opencontainers.image.licenses="MIT"

RUN apk add --no-cache ca-certificates tzdata postgresql17-client gzip \
    && addgroup -S app \
    && adduser -S -D -H -h /app -s /sbin/nologin -G app app \
    && mkdir -p /app /translations /web-app/dist /backups \
    && chown -R app:app /app /translations /web-app /backups

COPY --from=builder /bin/app /app/app

COPY --from=builder /app/db /db
COPY --from=builder /app/translations /translations

# Include the built frontend
COPY --from=frontend /frontend/dist /web-app/dist

USER app

ENV DISABLE_ENV_FILE=true

CMD ["/app/app"]

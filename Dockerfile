# syntax=docker/dockerfile:1

# Beide Images werden aus dem Repository-Stamm gebaut:
# docker build --target edge -t kuzey-ruzgarlari-edge .
# docker build --target api  -t kuzey-ruzgarlari-api  .

# Node.js 26: aktuelle LTS-Version. Go, Caddy und Alpine: aktuelle stabile Versionen.
FROM node:26.13.0-alpine3.24 AS frontend-build
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY frontend/ ./
RUN npm run build -- --configuration production \
    && test -f dist/kuzey-ruzgarlari/browser/index.html \
    && mkdir -p /out/web \
    && cp -a dist/kuzey-ruzgarlari/browser/. /out/web/

FROM golang:1.27.3-alpine3.24 AS backend-build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY backend/ ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.24.3 AS api
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app
COPY --from=backend-build --chown=10001:10001 /out/api /usr/local/bin/api
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]

FROM caddy:2.11.4-alpine AS edge
ENV XDG_CONFIG_HOME=/tmp/caddy/config \
    XDG_DATA_HOME=/tmp/caddy/data
COPY ops/Caddyfile /etc/caddy/Caddyfile
COPY --from=frontend-build /out/web/ /srv/
USER 10001:10001
EXPOSE 8080

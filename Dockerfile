# syntax=docker/dockerfile:1

# Die drei Images werden aus dem Repository-Stamm gebaut:
# docker build --target edge -t nordwinds-edge .
# docker build --target api  -t nordwinds-api  .
# docker build --target ssr  -t nordwinds-ssr  .

# Node.js 26: aktuelle LTS-Version. Go, Caddy und Alpine: aktuelle stabile Versionen.
FROM node:26.13.0-alpine3.24 AS frontend-build
WORKDIR /src/frontend
RUN npm install --global npm@12.2.0
COPY frontend/package.json frontend/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY frontend/ ./
RUN npm run build -- --configuration production \
    && test -f dist/nordwinds/browser/index.csr.html \
    && test -f dist/nordwinds/server/server.mjs

FROM golang:1.27.3-alpine3.24 AS backend-build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY backend/ ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM alpine:3.24.3 AS api
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app
COPY --from=backend-build --chown=10001:10001 /out/api /usr/local/bin/api
COPY --from=backend-build --chown=10001:10001 /out/migrate /usr/local/bin/migrate
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]

FROM node:26.13.0-alpine3.24 AS ssr
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm install --global npm@12.2.0 \
    && npm ci --omit=dev
COPY --from=frontend-build /src/frontend/dist/nordwinds ./dist/nordwinds
ENV NODE_ENV=production
USER node
EXPOSE 4000
CMD ["node", "dist/nordwinds/server/server.mjs"]

FROM caddy:2.11.4-alpine AS edge
ENV XDG_CONFIG_HOME=/tmp/caddy/config \
    XDG_DATA_HOME=/tmp/caddy/data
COPY ops/Caddyfile /etc/caddy/Caddyfile
USER 10001:10001
EXPOSE 8080

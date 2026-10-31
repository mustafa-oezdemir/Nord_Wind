#!/usr/bin/env sh
set -eu

base_url="${1:-http://127.0.0.1:8080}"
base_url="${base_url%/}"

for service in db api ssr edge; do
  container_id="$(docker compose ps -q "$service")"
  test -n "$container_id"
  health="starting"
  attempt=0
  while [ "$health" = starting ] && [ "$attempt" -lt 15 ]; do
    health="$(docker inspect --format '{{.State.Health.Status}}' "$container_id")"
    [ "$health" = healthy ] && break
    attempt=$((attempt + 1))
    sleep 2
  done
  test "$health" = healthy
done

curl -fsS "${base_url}/" | grep -E '<title>[^<]*NordWinds</title>' >/dev/null
curl -fsS "${base_url}/api/v1/meta" | grep -F '"name":"NordWinds"' >/dev/null

products_json="$(curl -fsS "${base_url}/api/v1/products?limit=1")"
slug="$(printf '%s' "$products_json" | sed -n 's/.*"slug":"\([^"]*\)".*/\1/p')"
if [ -n "$slug" ]; then
  detail_html="$(curl -fsS "${base_url}/produkte/${slug}")"
  printf '%s' "$detail_html" | grep -F 'rel="canonical"' >/dev/null
  printf '%s' "$detail_html" | grep -F 'detail-title' >/dev/null
fi

docker compose exec -T api /usr/local/bin/migrate status >/dev/null
printf '%s\n' 'Smoke-Test bestanden: Startseite, Produkt-SSR, Gin-API, Container-Health und Migration.'

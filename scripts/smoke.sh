#!/usr/bin/env sh
set -eu

base_url="${1:-http://127.0.0.1:8080}"
base_url="${base_url%/}"

for service in db api edge; do
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

curl -fsS "${base_url}/" | grep -F '<title>NordWinds</title>' >/dev/null
curl -fsS "${base_url}/api/v1/meta" | grep -F '"name":"NordWinds"' >/dev/null

docker compose exec -T api /usr/local/bin/migrate status >/dev/null
printf '%s\n' 'Smoke-Test bestanden: Startseite, Gin-API, Container-Health und Migration.'

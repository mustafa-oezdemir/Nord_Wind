#!/bin/sh
set -eu

umask 077
if [ ! -d /backups ] || [ ! -w /backups ]; then
  printf '%s\n' '{"event":"backup.failed","reason":"backup_directory_unwritable"}' >&2
  exit 1
fi

backup_once() {
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  suffix="$(od -An -N8 -tx1 /dev/urandom | tr -d ' \n')"
  base="nordwinds-${stamp}-${suffix}"
  partial="/backups/${base}.dump.partial"
  dump="/backups/${base}.dump"
  checksum="/backups/${base}.sha256"
  trap 'rm -f "$partial"' EXIT HUP INT TERM
  if ! pg_dump --format=custom --no-owner --no-acl --file="$partial"; then
    rm -f "$partial"
    trap - EXIT HUP INT TERM
    printf '%s\n' '{"event":"backup.failed","reason":"pg_dump"}' >&2
    return 1
  fi
  if ! pg_restore --list "$partial" >/dev/null; then
    rm -f "$partial"
    trap - EXIT HUP INT TERM
    printf '%s\n' '{"event":"backup.failed","reason":"archive_invalid"}' >&2
    return 1
  fi
  if ! mv "$partial" "$dump"; then
    rm -f "$partial"
    trap - EXIT HUP INT TERM
    printf '%s\n' '{"event":"backup.failed","reason":"move"}' >&2
    return 1
  fi
  if ! (cd /backups && sha256sum "${base}.dump" > "${base}.sha256" && sha256sum -c "${base}.sha256" >/dev/null); then
    rm -f "$dump" "$checksum"
    trap - EXIT HUP INT TERM
    printf '%s\n' '{"event":"backup.failed","reason":"checksum"}' >&2
    return 1
  fi
  touch /backups/.last-success
  printf '{"event":"backup.success","file":"%s.dump","bytes":%s}\n' "$base" "$(wc -c < "$dump" | tr -d ' ')"
  trap - EXIT HUP INT TERM
}

if [ "${NORDWINDS_BACKUP_ONCE:-false}" = true ]; then
  backup_once
  exit $?
fi

while :; do
  if ! backup_once; then
    printf '%s\n' '{"event":"backup.failed","reason":"backup_cycle"}' >&2
  fi
  sleep 86400
done

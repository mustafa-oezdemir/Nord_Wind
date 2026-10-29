# NW-01: Verifizierte Ausgangslage

Stand: 29. Oktober 2026. Arbeitsbasis: Commit `ba278fcb7aecc4a41324ef7592bdf87eb320b49a` auf `main`; die Prüfung erfolgte auf `feat/nw-01-baseline`. Der Fahrplan beschreibt einen möglichen MarktHaus-Starter, aber in diesem Checkout ist keiner vorhanden. Die vorhandene lokale `.env` wurde weder überschrieben noch veröffentlicht.

## Tatsächlich vorhandene Dateien

Versioniert sind `README.md`, `.env.example`, `.gitignore`, `.dockerignore`, `Dockerfile`, `ops/Caddyfile` und die Dokumentation unter `docs/`. Nicht vorhanden sind `frontend/`, `backend/`, `compose.yaml`, Migrationen, Seed-Daten, OpenAPI, ADRs und `scripts/smoke.sh`. Das Dockerfile verweist auf `frontend/package.json`, `frontend/package-lock.json`, `backend/go.mod` und `backend/go.sum`; diese Eingaben fehlen.

## Prüfungen und Ergebnisse

| Prüfung | Tatsächlicher Befehl oder Nachweis | Ergebnis | Beleg / Grenze |
| --- | --- | --- | --- |
| Git-Ausgangslage | `git status --short --branch`, `git rev-parse HEAD`, `git ls-remote origin refs/heads/main` | **PASS** | Arbeitsbaum vor der Dokumentationsänderung sauber; lokaler Basis-Commit und Remote-`main` waren beide `ba278fcb7aec`. |
| Docker Engine | `docker --version`, `docker info --format '{{.ServerVersion}}'` | **PASS** | Client und Server: `29.9.2`. |
| Docker Compose | `docker compose version` | **PASS** | Version `v5.6.1` installiert. |
| Docker Buildx | `docker buildx version` | **PASS** | Version `v0.38.1` installiert. |
| Lokale Toolchains | `go version`, `node --version`, `npm --version` | **PASS** | Go `1.26.5` auf Windows; Node `v26.13.0`; npm `12.2.0`. Das Dockerfile verwendet Go `1.27.3`; ein lokaler Go-Build wurde nicht ausgeführt. |
| Lokale Umgebung | Schlüsselvergleich `.env` gegen `.env.example`; `git check-ignore --no-index` | **PASS** | `.env` vorhanden, 17 passende Schlüssel und ein Nicht-Platzhalter-Passwort; Wert nicht ausgegeben. `.env`, `_todo/` und `AGENTS.md` werden ignoriert. |
| Dockerfile-Syntax und Ziele | `docker buildx build --call=targets --progress=plain -f Dockerfile .` | **PASS** | Buildx erkennt `frontend-build`, `backend-build`, `api` und `edge`; dies ist **kein** Image-Build. |
| Caddy-Konfiguration | `caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile` im offiziellen Caddy-Container | **PASS** | Ausgabe: `Valid configuration`. Die Konfiguration deaktiviert automatisches HTTPS und ist nur für lokale Entwicklung gedacht. |
| Compose-Konfiguration | `docker compose -f compose.yaml config --quiet` | **FAIL** | Exit-Code 1: `compose.yaml` kann nicht gefunden werden. |
| Containerstart und Health | `docker compose up --build -d`, `docker compose ps`, Logs und HTTP-Health | **NICHT AUSGEFÜHRT** | Ohne `compose.yaml`, Frontend und Backend gibt es keinen startbaren Stack. Keine Volumes wurden verändert. |
| Go-Tests und Migrationen | `go test ./...` unter `backend/`; Migration up/down/up | **NICHT AUSGEFÜHRT** | `backend/` und Migrationen fehlen. |
| Angular-Typecheck und Build | `npm run typecheck`, `npm run build` unter `frontend/` | **NICHT AUSGEFÜHRT** | `frontend/` und `package.json` fehlen; kein `npm install` ohne Projektdateien. |
| Smoke-Test und API-Vertrag | `scripts/smoke.sh`, OpenAPI-Abgleich | **NICHT AUSGEFÜHRT** | Skript, API und Vertrag fehlen. |

## Bewertung

Der Fehler beim Compose-Aufruf entsteht durch eine fehlende Anwendung, nicht durch eine nachgewiesene Fehlfunktion einer vorhandenen Anwendung. Ein vollständiger Docker-Build oder eine funktionierende Katalog-API darf daher noch nicht als bestanden gelten. Der ältere MarktHaus-Starter ist eine Beschreibung in einer Quelle und kein Bestandteil dieses Checkouts.

Die Basis ist für Dokumentationsarbeit nutzbar, **für Funktionsentwicklung und NW-02 aber noch nicht lauffähig**. Vor dem nächsten Feature-Schritt ist ein kleiner, überprüfbarer Starter erforderlich: Angular-Projekt mit Lockfile, Go-Modul mit Health-Endpunkten, PostgreSQL-/Compose-Verkabelung und ein Smoke-Test. Danach müssen Compose-Config, Image-Build, Health, Go-Tests und Angular-Build erneut tatsächlich ausgeführt werden. Diese Nacharbeit ist im Backlog als **NW-01a** erfasst.

## Sicherheits- und Betriebsgrenzen

- Die vorhandene `.env` bleibt lokal; weder Passwort noch private Planungsunterlagen wurden ausgegeben oder gestaged.
- `ops/Caddyfile` dient derzeit lokalem HTTP auf Port 8080. Öffentlicher Betrieb erfordert eine eigene HTTPS- und Sicherheitskonfiguration.
- Datenbank-Volumes wurden nicht angelegt oder gelöscht; es gab keinen Datenbankstart.
- Die Dockerfile-Ziele nutzen nicht-root Benutzer, aber ein laufender Container wurde noch nicht geprüft.

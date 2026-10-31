# Monitoring mit Prometheus und Grafana

- **Stand:** 31. Oktober 2026. Code, Konfiguration und Tests sind fertig. Der Lauf mit Docker steht noch aus (siehe [Prüfungen](#prüfungen)).
- **Basis:** `main` mit E-Mail-Benachrichtigungen, Mailpit-Volume, Trivy-Pin, Header und Verkaufsanalyse im Adminbereich (`6b85b16`).
- **Entscheidung:** [ADR-012](../adr/ADR-012-monitoring.md).
- **Vorgaben des Projektinhabers:**
  - Prometheus und Grafana einrichten.
  - TLS für Prometheus-API und -Oberfläche ([Prometheus-Leitfaden](https://prometheus.io/docs/guides/tls-encryption/)).
  - Benutzer und Passwörter in `.env`.
  - Alles überwachen, was eine Anwendung braucht.
  - Registrierung nicht verändern.

## Starten

In `.env` stehen die Zugangsdaten und `COMPOSE_PROFILES=monitoring`; auf dem Rechner des Projektinhabers ist das bereits eingetragen. Dann genügt:

```powershell
docker compose up --build -d   # baut nordwinds-monitoring-init, legt die DB-Rolle an, startet alles
docker compose ps              # prometheus „healthy“, monitoring-init „exited (0)“
```

`docker compose down -v` nicht verwenden: Es löscht auch Datenbank, Mailpit und die Monitoring-Daten.

| Oberfläche | Adresse | Anmeldung (Werte in `.env`) |
| --- | --- | --- |
| Grafana | http://127.0.0.1:3000 | `GRAFANA_ADMIN_USER` / `GRAFANA_ADMIN_PASSWORD` |
| Prometheus | https://127.0.0.1:9090 | `PROMETHEUS_USER` / `PROMETHEUS_PASSWORD` |

Grafana öffnet direkt das Dashboard **NordWinds – Betrieb**. Prometheus zeigt unter `/targets` die Ziele, unter `/alerts` die Alarme und unter `/rules` den Zustand jeder Regel.

### Browserwarnung bei Prometheus

Das Zertifikat stammt von der lokalen CA „NordWinds Monitoring CA (lokal)“. Der Browser warnt deshalb einmal; die Warnung kann bestätigt werden. Ohne Warnung geht es, wenn die CA nur für den eigenen Benutzer vertraut wird. Sie ist auf `prometheus`, `localhost`, `127.0.0.1` und `::1` beschränkt (ADR-012) und kann keine anderen Websites ausweisen.

```powershell
docker compose cp prometheus:/run/monitoring/ca.crt certs/local/nordwinds-monitoring-ca.crt
Import-Certificate -FilePath certs/local/nordwinds-monitoring-ca.crt -CertStoreLocation Cert:\CurrentUser\Root
```

`docker compose logs monitoring-init` nennt den SHA-256-Fingerabdruck (`ca_sha256`). Windows zeigt ihn vor dem Import an, sodass sich die Datei vergleichen lässt.

## Dashboard „NordWinds – Betrieb“

| Bereich | Inhalt |
| --- | --- |
| Verfügbarkeit | Shop, Katalog-API und PostgreSQL erreichbar; nicht erreichbare Ziele; aktive Alarme; Laufzeit der API |
| API-Verkehr | Anfragen je Statusklasse, 5xx-Anteil, p50/p95/p99, Fehler je Route, langsamste und meistgenutzte Routen |
| Shop | Bestellungen gesamt und je Status, bezahlte Bestellwerte, aktive Kundenkonten, Reservierungen, ausverkaufte Varianten, Zahlungsereignisse |
| E-Mail-Benachrichtigungen | wartend, endgültig fehlgeschlagen, älteste offene E-Mail, zugestellt, übersprungen, Verlauf je Zustand |
| Sicherheit | Anmeldeversuche je Ergebnis, abgewiesene Anfragen (4xx) je Route |
| Datenbank | Größe, Verbindungen und `max_connections`, Cache-Trefferquote, Deadlocks, längste Transaktion, Verbindungspool der API, Transaktionen, Abfrageprobleme |
| API-Prozess | Arbeitsspeicher, CPU, Goroutinen und Speicherbereinigung |
| Alarme und Monitoring | feuernde Alarme, Restlaufzeit des Prometheus-Zertifikats, Dauer des API-Abrufs |

Das Dashboard ist schreibgeschützt eingerichtet. Änderungen gehören in `ops/monitoring/grafana/dashboards/nordwinds-betrieb.json`; Grafana übernimmt sie nach höchstens 60 Sekunden.

## Alarme

28 Regeln in `ops/monitoring/prometheus/rules/nordwinds.yml`; `critical` bedeutet, dass Kunden betroffen sind.

| Alarm | Stufe | Bedingung |
| --- | --- | --- |
| NordWindsZielNichtErreichbar | critical | ein Scrape-Ziel 2 min nicht erreichbar |
| NordWindsShopNichtErreichbar | critical | Startseite oder `/produkte` 2 min ohne Erfolg |
| NordWindsKatalogApiNichtErreichbar | critical | `GET /api/v1/products` über Caddy 2 min ohne Erfolg |
| NordWindsPostgresNichtErreichbar | critical | Exporter 1 min ohne Verbindung |
| NordWindsHoheFehlerquote | critical | über 5 % 5xx über 5 min (bei mindestens 3 Anfragen pro Minute) |
| NordWindsDatenbankpruefungFehlgeschlagen | critical | `/health/ready` meldet Datenbankfehler |
| NordWindsApiStartetWiederholt | warning | mehr als 2 Neustarts in 30 min |
| NordWindsGeschaeftszahlenFehlen | warning | Geschäftskennzahlen 5 min nicht lesbar |
| NordWindsLangsameAntworten | warning | p95 aller Routen über 1 s für 10 min |
| NordWindsLangsameRoute | warning | p95 einer Route über 2 s für 10 min |
| NordWindsShopLangsam | warning | Shopseite über 3 s für 10 min |
| NordWindsDatenbankabfragenFehlerhaft | warning | mehr als 10 fehlgeschlagene Abfragen in 10 min |
| NordWindsLangsameAbfragen | warning | mehr als 5 Abfragen über 1 s in 10 min |
| NordWindsVerbindungspoolAusgelastet | warning | Wartezeit auf den Pool über 100 ms/s für 5 min |
| NordWindsPostgresVerbindungenKnapp | warning | über 80 % von `max_connections` für 5 min |
| NordWindsPostgresDeadlocks | warning | Deadlock in 15 min |
| NordWindsLangeTransaktion | warning | Transaktion über 5 min |
| NordWindsEmailEndgueltigFehlgeschlagen | warning | mindestens eine E-Mail im Zustand `failed` |
| NordWindsEmailStau | warning | älteste offene E-Mail über 15 min |
| NordWindsReservierungenUeberfaellig | warning | abgelaufene Reservierung über 10 min nicht freigegeben |
| NordWindsZahlungsfehler | warning | mehr als 3 abgelehnte Zahlungsereignisse in 10 min |
| NordWindsVieleFehlgeschlageneAnmeldungen | warning | mehr als 30 Fehlversuche oder Sperren in 10 min |
| NordWindsApiSpeicherHoch | warning | RSS über 512 MiB für 15 min |
| NordWindsApiGoroutinenHoch | warning | über 2000 Goroutinen für 15 min |
| NordWindsApiDateideskriptorenKnapp | warning | über 80 % der Dateideskriptoren für 10 min |
| NordWindsMonitoringZertifikatLaeuftAb | warning | Prometheus-Zertifikat läuft in unter 14 Tagen ab |
| NordWindsRegelauswertungFehlerhaft | warning | Regelauswertung fehlgeschlagen |
| NordWindsArtikelAusverkauft | info | ausverkaufte Variante seit 30 min |

Eine Zustellung (Alertmanager) gibt es noch nicht; siehe ADR-012.

## Zugangsdaten ändern

- **Prometheus-Passwort:** In `.env` ändern, dann `docker compose up -d`. `monitoring-init` schreibt den neuen Hash; Prometheus liest ihn sofort. Grafana bekommt die neue Datenquelle mit `docker compose restart grafana`.
- **Grafana-Passwort:** Grafana übernimmt `.env` nur beim ersten Start. Danach im Grafana-Profil „Passwort ändern“ verwenden und den neuen Wert zur Erinnerung auch in `.env` eintragen.
- **Datenbank-Monitorrolle:** In `.env` ändern, dann `docker compose up -d`. `migrate` setzt das Passwort neu, der Exporter verbindet sich mit dem neuen Wert.
- **Metrik-Token:** In `.env` ändern, dann `docker compose up -d`. API und Prometheus erhalten den neuen Wert gemeinsam.

## Fehlersuche

| Beobachtung | Ursache und Abhilfe |
| --- | --- |
| `monitoring-init` endet mit Fehler | `docker compose logs monitoring-init` nennt die Variable und die Regel (Werte werden nie ausgegeben), z. B. fehlendes `NORDWINDS_METRICS_TOKEN`. |
| Ziel `nordwinds-api` „down“, 401 | `NORDWINDS_METRICS_TOKEN` ist leer oder die API wurde nicht neu gestartet: `docker compose up -d`. |
| Ziel `postgres` meldet `pg_up 0` | Rolle fehlt, weil `NORDWINDS_DB_MONITOR_PASSWORD` beim letzten `migrate` leer war: `docker compose up -d` startet `migrate` erneut. |
| Shopprüfung schlägt fehl, Seite im Browser geht | Das SSR akzeptiert nur die Hosts `localhost` und `127.0.0.1`; die Prüfung sendet `Host: localhost:8080`. Wer `HOST_HTTP_PORT` oder die öffentliche Adresse ändert, passt `ops/monitoring/blackbox/blackbox.yml` an. |
| Port 3000 oder 9090 belegt | `HOST_GRAFANA_PORT` bzw. `HOST_PROMETHEUS_PORT` in `.env` ändern. |

Die Prometheus-Konfiguration nimmt `api:8080` an. Bei einem anderen `API_HTTP_PORT` ist `ops/monitoring/prometheus/prometheus.yml` anzupassen.

## Geänderte und neue Dateien

- `backend/internal/server/metrics.go`, `metrics_export.go`, `metrics_business.go`: Metriken je Route, HELP/TYPE, Pool, Laufzeit, Anmeldeversuche, Geschäftskennzahlen. Die bisherigen Metrik-Namen aus NW-24 bleiben erhalten.
- `backend/internal/server/auth.go`: vier Zählaufrufe in der Anmeldung. Die Registrierung ist unverändert.
- `backend/internal/migrations/monitor_role.go`, `backend/cmd/migrate/main.go`: Datenbankrolle für den Exporter.
- `backend/internal/monitoring/`, `backend/cmd/monitoring-init/`: CA, TLS, bcrypt, Grafana-Datenquelle.
- `Dockerfile`: Ziel `monitoring-init` (`FROM scratch`).
- `compose.yaml`: Profil `monitoring` mit fünf Diensten, Netz `monitoring`, fünf Volumes; Rollenvariablen für `migrate`.
- `ops/monitoring/`: `prometheus/prometheus.yml`, `prometheus/rules/nordwinds.yml`, `blackbox/blackbox.yml` und `grafana/dashboards/nordwinds-betrieb.json`.
- `.env.example`, `README.md`, `backend/README.md`, `.github/workflows/ci.yml` (Prüfung mit `promtool` und `--config.check`).
- Nicht geändert: Registrierung, Frontend, Caddy, Bestell-, Zahlungs- und E-Mail-Code, Staging-Konfiguration.

## Prüfungen

| Prüfung | Ergebnis |
| --- | --- |
| `gofmt -l`, `gofmt -s -l`, `go vet ./...` (auch mit `GOOS=windows`), `go build ./...` | **PASS** |
| Textformat des Metrik-Endpunkts: HELP und TYPE vor den Werten, Familien zusammenhängend, kumulative Buckets, Labels nur aus Routenmustern, `OTHER`/`unmatched` | **PASS** |
| Anmeldeversuche: nur bekannte Ergebnisse, keine Adressen | **PASS** |
| `TestBusinessMetricsOnPostgres`: leere Datenbank mit Nullwerten, Konto mit Begrüßung in der Outbox (Alter der ältesten E-Mail), ausverkaufte Variante, Pool-Werte, keine Kunden- oder Produktdaten, Datenbankausfall mit `up 0` | **PASS** |
| `TestMonitorRoleOnPostgres`: ungültige Namen, kurzes Passwort und Eigentümerrolle abgelehnt; zweimal ausführbar; `pg_monitor`, kein Superuser, Limit 3; Statistiken lesbar; `customers`, `orders` und `CREATE TABLE` verweigert (42501); Passwort mit `'` und `$` | **PASS** |
| `monitoring-init`: fehlende, schwache und Platzhalterwerte mit Variablennamen ohne Wert gemeldet; Rechte 0400/0444/0600/0700 und Eigentümer 65534/472 (als root); bcrypt-Hash passt; TLS-Handshake für `prometheus`, `localhost`, `127.0.0.1`; fremder Name abgelehnt; Namensbeschränkung der CA wirkt; `$` und `'` in der Grafana-Datenquelle maskiert; zweiter Lauf ohne Erneuerung; Erneuerung kurz vor Ablauf; beschädigter Schlüssel ersetzt | **PASS** |
| `go test ./... -count=1` mit getrennten Datenbanken `nordwinds_test_*` | **PASS** außer dem bekannten NW-07-Test in `internal/catalog` (PostgreSQL 16 meldet 23503 statt 23001, siehe [NW-10](NW-10.md)) |
| `go test -race` für `server`, `monitoring`, `migrations` | **PASS** |
| Echte API-Binärdatei gegen PostgreSQL: `migrate up` legt die Rolle an; `/internal/metrics` mit 42 Familien, 401 ohne Token, Routen-, Anmelde-, Pool-, Prozess- und Geschäftswerte; weder E-Mail-Adresse noch Token in der Ausgabe | **PASS** |
| `monitoring-init` statisch gelinkt (`FROM scratch` geeignet) | **PASS** |
| Konsistenz: alle 89 PromQL-Ausdrücke (Regeln und Dashboard) mit ausgeglichenen Klammern; jede `nordwinds_*`-, `go_*`- und `process_*`-Metrik existiert im Code; jede Aufzeichnungsregel ist definiert; Dashboard ohne Überlappung, eindeutige IDs | **PASS** |
| `promtool check config/rules`, `blackbox_exporter --config.check`, `docker compose --profile monitoring config`, Image-Bau | **NICHT AUSGEFÜHRT**: Hier gibt es kein Docker, und heruntergeladene Programme dürfen nicht ausgeführt werden. Die Prüfungen laufen in der CI (Schritt „Monitoring-Konfiguration prüfen“). |
| Lauf mit Docker: Ziele „up“, Dashboard mit Daten, Anmeldung an Prometheus über HTTPS | **NICHT AUSGEFÜHRT**; steht nach `docker compose up --build -d` auf dem Rechner des Projektinhabers aus |

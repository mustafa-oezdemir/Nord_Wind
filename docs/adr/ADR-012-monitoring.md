# ADR-012 — Monitoring mit Prometheus und Grafana

- **Status:** Angenommen am 31. Oktober 2026 (Auftrag des Projektinhabers: „Prometheus und Grafana einrichten, TLS für die Prometheus-API und -Oberfläche, Zugangsdaten in `.env`, alles überwachen, was eine Anwendung braucht“).
- **Bezug:** [NW-24](../progress/NW-24.md) (Metrik-Endpunkt und vorgesehene Alarme), [ADR-011](ADR-011-e-mail-benachrichtigungen.md) (E-Mail-Outbox), [ADR-010](ADR-010-bestandsreservierung.md) (Reservierungen).
- **Umsetzung und Prüfungen:** [Monitoring](../progress/monitoring.md).

## Kontext

Seit NW-24 bietet die API `/internal/metrics` mit Bearer-Token an. Die Werte waren grob: Statusklassen und fünf Dauergrenzen über alle Routen. Es gab niemanden, der sie abfragte, keine Alarme und keine Übersicht. NW-24 nennt als erste Alarme: 5xx-Anstieg, hohe Latenz, Datenbank- und Abfragefehler, Zahlungsfehler und eine zu alte Sicherung.

## Entscheidung

Ein Compose-Profil `monitoring` mit fünf Diensten. Ohne das Profil startet nichts davon; CI und Staging bleiben unverändert.

| Dienst | Image | Aufgabe |
| --- | --- | --- |
| `monitoring-init` | eigenes Image `monitoring-init` (Go, `FROM scratch`) | Erzeugt CA, TLS-Zertifikat, bcrypt-Hash und Dateien mit Zugangsdaten; beendet sich danach. |
| `prometheus` | `prom/prometheus:v3.13.4` (LTS-Linie 3.13) | Sammelt alle 15 s, wertet Regeln aus, speichert 15 Tage (höchstens 2 GB). |
| `postgres-exporter` | `prometheuscommunity/postgres-exporter:v0.20.1` | PostgreSQL-Statistiken mit einer Rolle ohne Zugriff auf Shopdaten. |
| `blackbox-exporter` | `prom/blackbox-exporter:v0.29.0` | Prüft den Shop von außen und die Laufzeit des eigenen TLS-Zertifikats. |
| `grafana` | `grafana/grafana:13.2.4` | Dashboard „NordWinds – Betrieb“, Datenquelle fest eingerichtet. |

### Was überwacht wird

Die Auswahl folgt den vier „goldenen Signalen“ (Verkehr, Fehler, Latenz, Auslastung) für jede Schicht und ergänzt die Abläufe, die im Projekt tatsächlich existieren.

| Bereich | Quelle | Kennzahlen |
| --- | --- | --- |
| Erreichbarkeit aus Kundensicht | Blackbox über Caddy | Startseite und `/produkte` (SSR, erwarteter Inhalt), `GET /api/v1/products`, Antwortzeit |
| API-Verkehr | API | Anfragen je Methode, **Routenmuster** und Statusklasse; Dauer-Histogramm mit 11 Grenzen (5 ms bis 10 s) je Route; laufende Anfragen |
| API-Prozess | API (`runtime/metrics`, `/proc`) | Goroutinen, Heap, belegter Speicher, GC, RSS, CPU, Dateideskriptoren, Startzeit |
| Datenbankzugriff der API | API (`sql.DBStats`) | offene, belegte, freie Verbindungen, Wartezeit auf den Pool |
| Datenbank | postgres-exporter | Erreichbarkeit, Verbindungen je Zustand und `max_connections`, Transaktionen, Rollbacks, Cache-Trefferquote, Deadlocks, Größe, längste Transaktion |
| Shop | API, aus PostgreSQL gelesen | Bestellungen und Bruttowerte je Status, Kundenkonten, simulierte Zahlungen je Zustand, aktive und überfällige Reservierungen, ausverkaufte Varianten |
| E-Mail | API, aus PostgreSQL gelesen | Outbox je Zustand, Alter der ältesten offenen E-Mail |
| Sicherheit | API | Anmeldeversuche je Ergebnis (Erfolg, falsches Passwort, Sperre, Fehler), abgewiesene Anfragen je Route |
| Zahlungen | API (seit NW-24) | Ereignisse je Ergebnis, abgelehnte Ereignisse |
| Monitoring selbst | Prometheus, Blackbox | Erreichbarkeit aller Ziele, Regelfehler, Restlaufzeit des TLS-Zertifikats |

Geschäftskennzahlen liest die API bei jedem Abruf mit einer einzigen Abfrage aus PostgreSQL (Zeitlimit 2 s). Dadurch stimmen sie nach Neustarts und bei mehreren API-Instanzen. Zähler im Prozess wären nach jedem Neustart wieder bei null und würden Abläufe außerhalb der API (Trigger der Outbox) nicht sehen. Fällt die Datenbank aus, meldet `nordwinds_business_metrics_up` den Wert 0; der Abruf selbst gelingt weiter.

Healthchecks (`/health/*`) und Metrikabrufe (`/internal/metrics`) werden mitgezählt, aber aus allen Kundenkennzahlen und Alarmen herausgefiltert. Sonst würden die Docker-Healthchecks im Ruhezustand Fehlerquote und Latenz bestimmen.

### Alarme

Prometheus wertet 28 Alarmregeln und 4 Aufzeichnungsregeln aus `ops/monitoring/prometheus/rules/nordwinds.yml` aus. Die Liste mit Schwellen steht im [Umsetzungsdokument](../progress/monitoring.md#alarme). Die Schwellen sind Startwerte für die lokale Vorschau; vor einem Livebetrieb sind sie mit echten Lastdaten zu prüfen.

Alarme sind unter `/alerts` in Prometheus und im Dashboard sichtbar. Eine Zustellung per E-Mail oder Chat (Alertmanager) ist bewusst nicht eingerichtet: Es gibt noch keinen Empfänger, keine Bereitschaft und keinen Livebetrieb. Lokal würde jede Benachrichtigung ohnehin nur in Mailpit landen.

### Sicherheit

- **TLS für Prometheus:** Oberfläche und API sprechen nur HTTPS (mindestens TLS 1.2) über `--web.config.file`. Das Zertifikat (ECDSA P-256, 397 Tage) stellt eine eigene lokale CA aus (10 Jahre). `monitoring-init` erneuert es bei jedem Start, sobald es in weniger als 30 Tagen abläuft. Die Blackbox-Prüfung warnt 14 Tage vorher.
- **Namensbeschränkte CA:** Die CA darf nur Zertifikate für `prometheus`, `localhost`, `127.0.0.1` und `::1` ausstellen (kritische X.509-Erweiterung *Name Constraints*). Wer sie in den Windows-Zertifikatspeicher übernimmt, um die Browserwarnung zu vermeiden, macht damit keine fremde Website angreifbar, selbst wenn der CA-Schlüssel abhandenkäme.
- **Anmeldung:** Prometheus verlangt Basic Auth. Gespeichert wird nur ein bcrypt-Hash; das Passwort steht ausschließlich in `.env`. Prometheus liest die Web-Konfiguration bei jeder Anfrage, ein neues Passwort gilt nach `docker compose up -d` sofort. Grafana hat nur das Admin-Konto aus `.env`; Selbstregistrierung und anonymer Zugriff sind aus.
- **Nur lokal erreichbar:** Prometheus (`127.0.0.1:9090`) und Grafana (`127.0.0.1:3000`) sind nur am eigenen Rechner veröffentlicht. Exporter und API-Metriken sind nur im Compose-Netz erreichbar; Caddy beantwortet `/internal/*` weiterhin mit 404.
- **Getrennte Geheimnisse:**
  - Der CA-Schlüssel liegt in einem Volume, das nur `monitoring-init` einbindet.
  - Prometheus (Benutzer `nobody`) und Grafana (Benutzer 472) bekommen je ein eigenes Volume. Schlüssel und Passwörter darin sind nur für den jeweiligen Dienst lesbar (Modus 0400).
  - Der Blackbox-Exporter sieht nur das öffentliche `ca.crt`.
  - `monitoring-init` läuft ohne Netzwerk, schreibgeschützt und nur mit der Capability `CHOWN`.
- **Datenbankrolle:** `migrate up` legt `NORDWINDS_DB_MONITOR_USER` an, wenn `NORDWINDS_DB_MONITOR_PASSWORD` (mindestens 32 Zeichen) gesetzt ist. Die Rolle hat:
  - Mitgliedschaft in `pg_monitor`;
  - kein Superuser- oder Rollenrecht;
  - kein `CREATE` im Schema `public`;
  - höchstens 3 Verbindungen.

  Tabellen der Anwendung (Kunden, Bestellungen) kann sie nicht lesen; ein Test prüft das mit SQLSTATE 42501.
- **Keine personenbezogenen Daten:** Labels sind fest oder Routenmuster (`/api/v1/orders/:orderNumber`, nie die Nummer). Unbekannte HTTP-Methoden werden als `OTHER` gezählt, nicht gefundene Pfade als `unmatched`. E-Mail-Adressen, Bestellnummern, Produktnamen, Tokens und Fehlertexte erscheinen in keiner Metrik; Tests prüfen das.

### Versionen

Gewählt sind die neuesten Versionen, die am 31. Oktober 2026 mindestens zwei Wochen alt waren, wie bei Mailpit in ADR-011:

| Komponente | Version | Veröffentlicht | Neuere Version |
| --- | --- | --- | --- |
| Prometheus | 3.13.4 (LTS) | 29.09.2026 | 3.13.5 und 3.15.1 vom 21.10.2026 enthalten Sicherheitskorrekturen (Go 1.27.3, `golang.org/x/net`) – **ab 4. November auf 3.13.5 wechseln** |
| Grafana | 13.2.4 | 13.10.2026 | 13.2.5 und 13.3.0 vom 27.10.2026 |
| postgres-exporter | 0.20.1 | 07.07.2026 | – |
| blackbox-exporter | 0.29.0 | 07.10.2026 | 0.29.1 vom 21.10.2026 |

### Nicht umgesetzt

- **Alertmanager:** Es gibt keinen Empfänger und keinen Livebetrieb. Für den Livebetrieb nachrüsten.
- **Container- und Hostmetriken (cAdvisor, node-exporter):** cAdvisor braucht Zugriff auf den Docker-Socket, also root-Rechte auf dem Host. Unter Docker Desktop würden beide nur die interne Linux-VM messen. Die API meldet ihren eigenen Speicher- und CPU-Verbrauch.
- **Alter der Datenbanksicherung:** Der Dienst `backup` schreibt nur eine Markierungsdatei, die Prometheus nicht lesen kann. Sein Docker-Healthcheck meldet weiterhin nach 26 Stunden „unhealthy“. Für Prometheus wäre ein Pushgateway nötig; das ist als nächster Schritt vorgesehen.
- **Metriken von Caddy und SSR:** Die Caddy-Konfiguration bleibt unverändert, um den Shop nicht zu gefährden. Die Blackbox-Prüfungen decken Erreichbarkeit und Antwortzeit von Caddy und SSR ab.
- **Bibliothek `prometheus/client_golang`:** Der vorhandene Endpunkt aus NW-24 ist von Hand geschrieben und braucht keine neue Abhängigkeit. Das Textformat wird in Tests streng geprüft (HELP/TYPE, zusammenhängende Familien, kumulative Buckets).
- **Staging:** Das Profil ist dort nicht eingeschaltet; dafür müssen die Zugangsdaten zuerst in `.env.staging` stehen.

## Folgen

- `COMPOSE_PROFILES=monitoring` in `.env` schaltet das Monitoring mit dem normalen `docker compose up -d` ein.
- `golang.org/x/crypto` ist jetzt eine direkte Abhängigkeit (bcrypt in `monitoring-init`). Vorher war es eine indirekte Abhängigkeit in derselben Version v0.58.0; `go.sum` bleibt unverändert.
- CI prüft die Prometheus-Konfiguration und die Regeln mit `promtool`, die Blackbox-Konfiguration mit `--config.check`, das Dashboard als JSON und baut das Image `monitoring-init`.
- Wer das Grafana-Passwort in `.env` ändert, muss es zusätzlich in Grafana zurücksetzen. Grafana übernimmt `GRAFANA_ADMIN_PASSWORD` nur beim ersten Start.

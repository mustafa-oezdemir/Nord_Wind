# NordWinds

Geplanter Onlineshop für den deutschen Markt mit deutschsprachiger Oberfläche und Preisen in Euro.

## Projektstand

Dieses Repository enthält eine deutschsprachige Angular-Storefront, eine Go-API mit Gin, versionierte PostgreSQL-Migrationen und einen lokalen Docker-Compose-Stack. Benutzerkonten, Warenkorb und eine Testkasse sind implementiert. Der Shop verkauft noch nichts; Betreiber, Sortiment und rechtliche Freigabe fehlen. Die verlinkten Rechtsseiten kennzeichnen diesen Vorschauzustand ausdrücklich.

## Geplante Architektur

- Angular, TypeScript und Bootstrap für die Kundenoberfläche; Node rendert öffentliche Katalogseiten serverseitig
- Go mit Gin-REST-API für die Serveranwendung
- PostgreSQL für die dauerhafte Datenspeicherung
- Docker Compose und Caddy für den lokalen Betrieb und die HTTP-Weiterleitung an Go-API und Angular-SSR

Die erste Version ist als Onlineshop eines einzelnen Anbieters geplant. Der Markenname lautet **NordWinds**. Für neue technische Bezeichner wird `nordwinds` verwendet.

## Lokale Konfiguration

Die Datei `.env.example` enthält freigebbare Variablennamen und Beispielwerte. Für die lokale Entwicklung wird sie nach `.env` kopiert. Anschließend muss `POSTGRES_PASSWORD` durch ein eigenes, starkes Passwort ersetzt werden. `.env` und der lokale Planungsordner `_todo/` werden nicht in Git aufgenommen.

Testbestellungen sind standardmäßig gesperrt. Ausschließlich für lokale Simulationen kann `NORDWINDS_DEMO_CHECKOUT_ENABLED=true` bei `APP_ENV=development` gesetzt werden. In jeder anderen Umgebung bleibt `POST /api/v1/orders` gesperrt. Diese Einstellung ist keine Freigabe für echte Verkäufe. Der [NW-21-Prüfstand](docs/progress/NW-21.md) nennt die fachlichen und rechtlichen Voraussetzungen.

## Lokal starten und prüfen

Die folgenden Befehle werden im Repository-Stamm ausgeführt. Die lokale `.env` benötigt vor dem ersten Start ein eigenes, starkes `POSTGRES_PASSWORD`.

```powershell
docker compose -f compose.yaml config --quiet
docker compose up --build -d
pwsh -File scripts/smoke.ps1
docker compose ps
```

Unter einer POSIX-Shell führt `sh scripts/smoke.sh` denselben Smoke-Test aus. `docker compose down` stoppt den Stack, ohne das Datenbank-Volume zu entfernen. Die Datenbank ist nicht am Host veröffentlicht; nur Caddy ist lokal unter `127.0.0.1:8080` erreichbar. Der Port kann über `HOST_HTTP_PORT` in `.env` geändert und dem Smoke-Test als URL übergeben werden.

Der `backup`-Dienst erstellt nach dem Start und danach täglich ein geprüftes Datenbankarchiv in `backups/`; der Ordner ist lokal und wird von Git ignoriert. Die [Rücksicherungsanleitung](docs/progress/NW-24.md) beschreibt den Test in einer separaten Datenbank. Für einen öffentlichen Betrieb sind verschlüsselte Kopien außerhalb des Hosts und eine Überwachung des Sicherungsstatus erforderlich.

Frontend und Backend lassen sich auch getrennt prüfen:

```powershell
cd frontend
npm ci
npm run typecheck
npm test -- --watch=false
npm run build
cd ../backend
go mod download
go test ./...
```

MailHog fängt Registrierungs-E-Mails im lokalen Entwicklungsbetrieb ab. Seine Oberfläche ist unter `http://127.0.0.1:8025` erreichbar; `HOST_MAILHOG_PORT` ändert den UI-Port. Der SMTP-Port ist nicht am Host veröffentlicht. Diese Nachrichten werden nicht an echte Postfächer zugestellt.

Der [Abgleich der Projektdokumente](docs/requirements/document-review.md) und der [priorisierte Backlog](docs/requirements/backlog.md) halten Anforderungen, Widersprüche und die nächsten Entwicklungsschritte fest.
Die [verifizierte Ausgangslage](docs/verification/baseline.md) zeigt, welche Prüfungen derzeit bestehen und welche wegen des fehlenden Starters noch nicht ausführbar sind.
Der [Stand der Markenänderung](docs/progress/NW-02.md) dokumentiert die Umstellung auf NordWinds und die damals noch ausstehenden Prüfungen.
Der [NW-03-Buildnachweis](docs/progress/NW-03.md) dokumentiert die inzwischen lauffähige Basis und ihre tatsächlichen Prüfergebnisse.
Die [CI- und PR-Anleitung](docs/verification/ci.md) erklärt die Qualitätsschritte und die Voraussetzungen für verpflichtende Statuschecks.
Der [NW-09-Nachweis](docs/progress/NW-09.md) beschreibt Storefront, Design-System und die ausgeführten bzw. noch ausstehenden Oberflächenprüfungen.
Der [NW-10-Nachweis](docs/progress/NW-10.md) beschreibt Produktdetail, Variantenauswahl und den neuen Detail-Endpunkt der API.
Der [NW-22-Nachweis](docs/progress/NW-22.md) dokumentiert die Zugänglichkeits- und SEO-Prüfung sowie die Entscheidung für gezieltes SSR.
Der [NW-23-Sicherheitsnachweis](docs/progress/NW-23.md) beschreibt die geprüften Schutzmaßnahmen, Scans und offenen Betriebsrisiken.
Der [NW-24-Betriebsnachweis](docs/progress/NW-24.md) beschreibt Metriken, tägliche Datenbanksicherung und die ausgeführte Rücksicherung.

## Container-Images

Das mehrstufige `Dockerfile` definiert drei Ziele: `edge` für Caddy, `ssr` für Angular und Node sowie `api` für Go und die Migration. Die Builds verwenden `npm ci`, `go mod download` und die versionierten Lock-/Prüfsummendateien. Compose startet PostgreSQL, führt Migrationen aus und schaltet danach API, SSR und Caddy frei. Caddy leitet `/api` an Go und die übrigen Anfragen an Angular weiter. Die Caddy-Konfiguration nutzt lokal HTTP auf Port 8080; für einen öffentlichen Betrieb ist eine gesonderte HTTPS-Konfiguration erforderlich.

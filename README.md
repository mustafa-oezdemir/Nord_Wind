# NordWinds

Geplanter Onlineshop für den deutschen Markt mit deutschsprachiger Oberfläche und Preisen in Euro.

## Projektstand

Dieses Repository enthält derzeit die grundlegende Konfiguration, Vorlagen für die Container-Images und den Anforderungsabgleich. Die Angular-Oberfläche, die Go-API, das PostgreSQL-Schema und die Docker-Compose-Konfiguration sind noch nicht implementiert. Der Shop ist noch nicht lauffähig.

## Geplante Architektur

- Angular, TypeScript und Bootstrap für die Kundenoberfläche
- Go mit REST-API für die Serveranwendung
- PostgreSQL für die dauerhafte Datenspeicherung
- Docker Compose und Caddy für den lokalen Betrieb und die HTTP-Weiterleitung

Die erste Version ist als Onlineshop eines einzelnen Anbieters geplant. Der Markenname lautet **NordWinds**. Für neue technische Bezeichner wird `nordwinds` verwendet.

## Lokale Konfiguration

Die Datei `.env.example` enthält freigebbare Variablennamen und Beispielwerte. Für die lokale Entwicklung wird sie nach `.env` kopiert. Anschließend muss `POSTGRES_PASSWORD` durch ein eigenes, starkes Passwort ersetzt werden. `.env` und der lokale Planungsordner `_todo/` werden nicht in Git aufgenommen.

Start- und Testbefehle werden hier dokumentiert, sobald die Anwendung und die Compose-Dateien vorhanden sind.

Der [Abgleich der Projektdokumente](docs/requirements/document-review.md) und der [priorisierte Backlog](docs/requirements/backlog.md) halten Anforderungen, Widersprüche und die nächsten Entwicklungsschritte fest.
Die [verifizierte Ausgangslage](docs/verification/baseline.md) zeigt, welche Prüfungen derzeit bestehen und welche wegen des fehlenden Starters noch nicht ausführbar sind.
Der [Stand der Markenänderung](docs/progress/NW-02.md) dokumentiert die Umstellung auf NordWinds und die noch nicht ausführbaren Prüfungen.

## Container-Images

Das mehrstufige `Dockerfile` definiert zwei Ziele: `edge` für Angular und Caddy sowie `api` für die Go-Anwendung. Beide Ziele erwarten Quellcode in `frontend/` beziehungsweise `backend/`. Ein vollständiger Image-Build ist erst möglich, wenn diese Anwendungen angelegt sind. Die Caddy-Konfiguration nutzt für die lokale Entwicklung HTTP auf Port 8080; für einen öffentlichen Betrieb ist eine gesonderte HTTPS-Konfiguration erforderlich.

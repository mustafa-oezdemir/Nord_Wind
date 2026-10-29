# Kuzey Rüzgarları

Geplanter Onlineshop für den deutschen Markt mit deutschsprachiger Oberfläche und Preisen in Euro.

## Projektstand

Dieses Repository enthält derzeit nur die grundlegende Konfiguration. Die Angular-Oberfläche, die Go-API, das PostgreSQL-Schema und die Docker-Compose-Konfiguration sind noch nicht implementiert. Der Shop ist noch nicht lauffähig.

## Geplante Architektur

- Angular, TypeScript und Bootstrap für die Kundenoberfläche
- Go mit REST-API für die Serveranwendung
- PostgreSQL für die dauerhafte Datenspeicherung
- Docker Compose und Caddy für den lokalen Betrieb und die HTTP-Weiterleitung

Die erste Version ist als Onlineshop eines einzelnen Anbieters geplant. Der Markenname lautet **Kuzey Rüzgarları**. Für technische Bezeichner wird `kuzey-ruzgarlari` verwendet.

## Lokale Konfiguration

Die Datei `.env.example` enthält freigebbare Variablennamen und Beispielwerte. Für die lokale Entwicklung wird sie nach `.env` kopiert. Anschließend muss `POSTGRES_PASSWORD` durch ein eigenes, starkes Passwort ersetzt werden. `.env` und der lokale Planungsordner `_todo/` werden nicht in Git aufgenommen.

Start- und Testbefehle werden hier dokumentiert, sobald die Anwendung und die Compose-Dateien vorhanden sind.

# NordWinds-API

Das Go-Modul enthält eine Gin-API mit `/health/live`, `/health/ready` und `/api/v1/meta`. Die erste PostgreSQL-Migration legt nur die technische Tabelle `app_settings` an. Produkt- und Bestelldaten folgen in späteren Schritten.

## Lokale Prüfung

```bash
go mod download
go test ./...
go build ./cmd/api
go build ./cmd/migrate
```

Die Container starten die Migration vor der API. `migrate status` prüft die angewendeten Versionen. `migrate up` ergänzt fehlende Versionen transaktional; `migrate down` nimmt genau die neueste Version zurück und verweigert das Entfernen nicht leerer Tabellen. Version 2 ergänzt Katalog-, Kunden- und Bestelltabellen, ohne `app_settings` oder bestehende Version-1-Daten zu ändern. Für die lokale Datenbankverbindung werden die Variablen `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER` und `POSTGRES_PASSWORD` benötigt. Der lokale Compose-Stack verwendet `POSTGRES_SSLMODE=disable` ausschließlich im privaten Entwicklungsnetz.

Der PostgreSQL-Integrationstest läuft nur mit `NORDWINDS_MIGRATION_TEST_URL`. Er verlangt eine separate, zunächst leere Datenbank mit Namen `nordwinds_test_*`; er darf nicht auf das Compose-Datenbank-Volume zeigen. Er prüft `up/down/up`, bestehende Version-1-Daten und Datenverlustsperren sowie Unique-, FK- und Check-Constraints. Ohne diese Variable wird nur dieser Integrationstest übersprungen.

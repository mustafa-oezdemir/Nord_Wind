# NordWinds-API

Das Go-Modul enthält eine Gin-API mit `/health/live`, `/health/ready` und `/api/v1/meta`. Die erste PostgreSQL-Migration legt nur die technische Tabelle `app_settings` an. Produkt- und Bestelldaten folgen in späteren Schritten.

## Lokale Prüfung

```bash
go mod download
go test ./...
go build ./cmd/api
go build ./cmd/migrate
```

Die Container starten die Migration vor der API. `migrate status` prüft die angewendete Version. Für die lokale Datenbankverbindung werden die Variablen `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER` und `POSTGRES_PASSWORD` benötigt. Der lokale Compose-Stack verwendet `POSTGRES_SSLMODE=disable` ausschließlich im privaten Entwicklungsnetz.

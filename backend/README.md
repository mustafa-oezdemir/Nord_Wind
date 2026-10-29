# NordWinds-API

Das Go-Modul enthält eine Gin-API mit `/health/live`, `/health/ready` und `/api/v1/meta`. Migration 1 legt `app_settings` an; Migration 2 ergänzt Katalog-, Kunden- und Bestelltabellen. `internal/catalog` stellt parametrisierte PostgreSQL-Abfragen für Kategorien, Produkte, Varianten und ihre Beziehungen bereit. Die aufrufende Schicht kann dafür eine `*sql.DB` oder eine eigene `*sql.Tx` übergeben; öffentliche Katalog-Endpunkte folgen in NW-08.

## Lokale Prüfung

```bash
go mod download
go test ./...
go build ./cmd/api
go build ./cmd/migrate
```

Die Container starten die Migration vor der API. `migrate status` prüft die angewendeten Versionen. `migrate up` ergänzt fehlende Versionen transaktional; `migrate down` nimmt genau die neueste Version zurück und verweigert das Entfernen nicht leerer Tabellen. Version 2 ergänzt Katalog-, Kunden- und Bestelltabellen, ohne `app_settings` oder bestehende Version-1-Daten zu ändern. Für die lokale Datenbankverbindung werden die Variablen `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER` und `POSTGRES_PASSWORD` benötigt. Der lokale Compose-Stack verwendet `POSTGRES_SSLMODE=disable` ausschließlich im privaten Entwicklungsnetz.

Der PostgreSQL-Integrationstest läuft nur mit `NORDWINDS_MIGRATION_TEST_URL`. Er verlangt eine separate, zunächst leere Datenbank mit Namen `nordwinds_test_*`; er darf nicht auf das Compose-Datenbank-Volume zeigen. Er prüft `up/down/up`, bestehende Version-1-Daten und Datenverlustsperren sowie Unique-, FK- und Check-Constraints. Ohne diese Variable wird nur dieser Integrationstest übersprungen.

Der Katalog-Repository-Test benötigt `NORDWINDS_CATALOG_TEST_URL` für eine **andere** PostgreSQL-Testdatenbank mit Namen `nordwinds_test_*` und ohne vorhandene Katalogdaten. Er wendet die Migrationen an, führt alle Fixtures in Transaktionen aus, rollt diese nach jedem Teiltest zurück und prüft, dass keine Katalogdatensätze zurückbleiben. Bei gleichzeitiger Ausführung von `go test ./...` dürfen `NORDWINDS_MIGRATION_TEST_URL` und `NORDWINDS_CATALOG_TEST_URL` nicht auf dieselbe Datenbank zeigen. Ohne Katalog-Test-URL wird nur dieser Integrationstest übersprungen; ein Mock ersetzt den PostgreSQL-Lauf nicht. Der Test prüft CRUD, Kategorie- und Variantenbeziehungen sowie PostgreSQL-SQLSTATE bei Constraint-Verstößen. `go test -race ./...` erfordert eine Go-Umgebung mit aktiviertem CGO und C-Compiler.

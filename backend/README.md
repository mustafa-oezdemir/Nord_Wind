# NordWinds-API

Das Go-Modul enthält eine Gin-API mit `/health/live`, `/health/ready`, `/api/v1/meta`, `/api/v1/products` und `/api/v1/categories`. Migration 1 legt `app_settings` an; Migration 2 ergänzt Katalog-, Kunden- und Bestelltabellen. Migration 3 ergänzt Trigram-Suchindizes. `internal/catalog` stellt parametrisierte PostgreSQL-Abfragen für Kategorien, Produkte, Varianten und ihre Beziehungen bereit. Die aufrufende Schicht kann dafür eine `*sql.DB` oder eine eigene `*sql.Tx` übergeben.

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

## Öffentlicher Katalog

`GET /api/v1/products` liefert veröffentlichte Produkte mit aktiven Varianten; `GET /api/v1/categories` liefert aktive Kategorien. Beide Endpunkte unterstützen `category` (Slug), `q`, `sort`, `page` und `limit`. `page` beginnt bei 1, `limit` liegt zwischen 1 und 100 (Standard 20). Die Antwort enthält immer `data` als Array und `pagination` mit `page`, `limit`, `total` und `total_pages`. Ein unbekannter oder inaktiver Kategorie-Slug ergibt 404, ungültige Parameter ergeben 400. Der vollständige Vertrag einschließlich Sortieroptionen steht in [OpenAPI](../docs/api/openapi.yaml).

Der HTTP-Integrationstest benötigt `NORDWINDS_CATALOG_API_TEST_URL` für eine weitere separate Datenbank `nordwinds_test_*` ohne Katalogdaten. Seine Fixtures werden transaktional zurückgesetzt. Ohne diese Variable wird nur dieser Test übersprungen. Für einen gleichzeitigen Gesamtlauf mit allen drei PostgreSQL-Integrationstests sind **drei verschiedene Testdatenbanken** nötig. Die produktive Compose-Datenbank darf für diese Tests nicht verwendet werden.

## HTTP-Fehler und Request-Logging

Fehlerantworten folgen [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html) mit `Content-Type: application/problem+json`. `type`, `title`, `status`, `detail`, `code` und `request_id` sind enthalten; der HTTP-Status und `status` stimmen überein. Die stabilen Typ-URIs unter `/api/v1/problems/:slug` liefern beim Aufruf eine kurze deutsche Beschreibung. `X-Request-ID` enthält für jede Anfrage eine neu erzeugte Kennung, die auch im Go-Request-Kontext zur Verfügung steht. Ein eingehender `X-Request-ID` wird nicht ungeprüft übernommen.

| Code | HTTP | Bedeutung |
| --- | ---: | --- |
| `INVALID_REQUEST` | 400 | Ungültige Eingabe |
| `NOT_FOUND` | 404 | Ressource nicht gefunden |
| `CONFLICT` | 409 | Fachlicher Zustandskonflikt |
| `UNAUTHENTICATED` | 401 | Anmeldung fehlt oder ist ungültig |
| `FORBIDDEN` | 403 | Berechtigung oder CSRF-Prüfung fehlt |
| `RATE_LIMITED` | 429 | Zu viele Login-Versuche |
| `METHOD_NOT_ALLOWED` | 405 | HTTP-Methode nicht zulässig |
| `BODY_TOO_LARGE` | 413 | Anfrageinhalt über 1 MiB |
| `URI_TOO_LONG` | 414 | Anfrageziel über 2048 Byte |
| `REQUEST_TIMEOUT` | 503 | Anfrage überschritt ihr Zeitlimit |
| `SERVICE_UNAVAILABLE` | 503 | Abhängiger Dienst nicht verfügbar |
| `INTERNAL_ERROR` | 500 | Unerwarteter Fehler oder Panic |

Neue Handler ordnen erwartete Fehler über `server.APIError` und `server.HandleError` zu. Unbekannte Fehler werden ohne ihren internen Text als `INTERNAL_ERROR` ausgegeben. Der Body wird durch `http.MaxBytesReader` begrenzt; Handler müssen Lesefehler an `server.HandleError` übergeben, damit auch Streaming-Bodies zuverlässig mit 413 beantwortet werden. Der Request-Kontext hat ein Limit von fünf Sekunden. Handler und Datenbankaufrufe müssen den Kontext beachten; ein bereits geschriebener Response kann nach Ablauf nicht ersetzt werden. Der HTTP-Server begrenzt Header auf 16 KiB und Lese- und Schreibzeiten auf zehn Sekunden.

Access-Logs werden als `slog`-JSON ausgegeben. Sie enthalten nur Zeitpunkt, Service/Umgebung, Ereignis, Request-ID, HTTP-Methode, normalisierte Route, Status, Dauer und gegebenenfalls Fehlercode. Normale 4xx-Antworten erscheinen als `INFO`, 5xx als `ERROR`. URL-Parameter, Header, Cookies, Bodies, SQL- und fremde Fehlertexte werden nicht protokolliert.

## Kundenanmeldung

`POST /api/v1/auth/register` erstellt Kundenkonten, `POST /api/v1/auth/login` stellt eine neue Session aus, `GET /api/v1/auth/session` liefert den aktuellen Kunden und CSRF-Token, `POST /api/v1/auth/logout` widerruft die Session. `GET /api/v1/customer/me` ist ausschließlich für die Kundenrolle freigegeben. Der [OpenAPI-Vertrag](../docs/api/openapi.yaml) beschreibt Antworten und Fehlercodes. Die Sicherheitsentscheidung einschließlich Passwort-Reset-Entwurf steht in [ADR-004](../docs/adr/ADR-004-authentication.md).

Für HTTPS-Umgebungen muss `APP_PUBLIC_ORIGIN` auf den exakten öffentlichen Origin gesetzt sein, etwa `https://shop.example.invalid`; `APP_ENV` darf dort nicht `development` sein. Die API startet dann nur mit einer HTTPS-Origin. Das Session-Cookie ist `Secure`, `HttpOnly`, `SameSite=Strict` und ohne `Domain`. Ausschließlich bei `APP_ENV=development` erlaubt die API ein lokales HTTP-Cookie für die Entwicklung. Schreibzugriffe mit Session benötigen `X-CSRF-Token` aus der Login- oder Session-Antwort. Adminrechte werden nicht über die öffentliche Registrierung vergeben.

Der Authentifizierungs-Integrationstest benötigt `NORDWINDS_AUTH_TEST_URL` für eine **separate** zunächst leere PostgreSQL-Datenbank mit Namen `nordwinds_test_*`. Er prüft Registrierung, falsche Passwörter, CSRF, Session-Fixation und -Ablauf, Rate-Limit, Rollenwechsel und Redaction. Ohne die Variable wird nur dieser Test übersprungen. Bei einem parallelen Gesamtlauf benötigt jeder PostgreSQL-Integrationstest eine eigene Datenbank.

## Geschützte Katalogverwaltung

Die Admin-API unter `/api/v1/admin` bietet Kategorie-, Produkt- und Varianten-CRUD, Produkt-Kategorie-Zuordnungen sowie `GET /api/v1/admin/audit` für die letzten 100 Änderungen. Alle Routen erfordern die Rolle `admin`; POST, PUT und DELETE zusätzlich den Session-CSRF-Token. Die Angular-Verwaltungsseite liegt unter `/admin`. Der [OpenAPI-Vertrag](../docs/api/openapi.yaml) enthält die einzelnen Routen und vollständigen Schreibfelder. Fehlende Datensätze liefern 404, ungültige Eingaben 400 und Unique-/FK-Konflikte 409.

Migration `0005_admin_audit` schreibt erfolgreiche Änderungen gemeinsam mit dem Katalogdatensatz in einer Transaktion. Für den PostgreSQL-HTTP-Test `NORDWINDS_ADMIN_TEST_URL` auf eine weitere zunächst leere Datenbank `nordwinds_test_*` setzen. Es wird kein Standard-Admin angelegt; die kontrollierte Rollenvergabe und die noch offenen Betriebsentscheidungen sind in [NW-12](../docs/progress/NW-12.md) beschrieben.

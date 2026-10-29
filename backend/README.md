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

## HTTP-Fehler und Request-Logging

Fehlerantworten folgen [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html) mit `Content-Type: application/problem+json`. `type`, `title`, `status`, `detail`, `code` und `request_id` sind enthalten; der HTTP-Status und `status` stimmen überein. Die stabilen Typ-URIs unter `/api/v1/problems/:slug` liefern beim Aufruf eine kurze deutsche Beschreibung. `X-Request-ID` enthält für jede Anfrage eine neu erzeugte Kennung, die auch im Go-Request-Kontext zur Verfügung steht. Ein eingehender `X-Request-ID` wird nicht ungeprüft übernommen.

| Code | HTTP | Bedeutung |
| --- | ---: | --- |
| `INVALID_REQUEST` | 400 | Ungültige Eingabe |
| `NOT_FOUND` | 404 | Ressource nicht gefunden |
| `CONFLICT` | 409 | Fachlicher Zustandskonflikt |
| `METHOD_NOT_ALLOWED` | 405 | HTTP-Methode nicht zulässig |
| `BODY_TOO_LARGE` | 413 | Anfrageinhalt über 1 MiB |
| `URI_TOO_LONG` | 414 | Anfrageziel über 2048 Byte |
| `REQUEST_TIMEOUT` | 503 | Anfrage überschritt ihr Zeitlimit |
| `SERVICE_UNAVAILABLE` | 503 | Abhängiger Dienst nicht verfügbar |
| `INTERNAL_ERROR` | 500 | Unerwarteter Fehler oder Panic |

Neue Handler ordnen erwartete Fehler über `server.APIError` und `server.HandleError` zu. Unbekannte Fehler werden ohne ihren internen Text als `INTERNAL_ERROR` ausgegeben. Der Body wird durch `http.MaxBytesReader` begrenzt; Handler müssen Lesefehler an `server.HandleError` übergeben, damit auch Streaming-Bodies zuverlässig mit 413 beantwortet werden. Der Request-Kontext hat ein Limit von fünf Sekunden. Handler und Datenbankaufrufe müssen den Kontext beachten; ein bereits geschriebener Response kann nach Ablauf nicht ersetzt werden. Der HTTP-Server begrenzt Header auf 16 KiB und Lese- und Schreibzeiten auf zehn Sekunden.

Access-Logs werden als `slog`-JSON ausgegeben. Sie enthalten nur Zeitpunkt, Service/Umgebung, Ereignis, Request-ID, HTTP-Methode, normalisierte Route, Status, Dauer und gegebenenfalls Fehlercode. Normale 4xx-Antworten erscheinen als `INFO`, 5xx als `ERROR`. URL-Parameter, Header, Cookies, Bodies, SQL- und fremde Fehlertexte werden nicht protokolliert.

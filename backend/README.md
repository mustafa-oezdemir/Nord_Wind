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

Für einen Betrieb außerhalb von `APP_ENV=development` braucht die API eine Datenbankrolle ohne Superuser-, Rollenverwaltungs- oder DDL-Rechte. Dafür `NORDWINDS_DB_RUNTIME_USER=nordwinds_runtime` und ein eigenes `NORDWINDS_DB_RUNTIME_PASSWORD` mit mindestens 32 Zeichen konfigurieren. `migrate up` legt die Rolle an und erteilt ihr die benötigten Tabellen- und Sequenzrechte; Migrationen verwenden weiter den Eigentümerzugang. Die API verweigert in anderen Umgebungen den Start mit einer erhöhten Datenbankrolle. Im lokalen Entwicklungsmodus bleiben die bisherigen Zugangsdaten zulässig. Die Rollenvergabe und die Grenzen dieser Prüfung sind im [NW-23-Sicherheitsnachweis](../docs/progress/NW-23.md) beschrieben.

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
| `RATE_LIMITED` | 429 | Zu viele Login- oder Registrierungsversuche |
| `METHOD_NOT_ALLOWED` | 405 | HTTP-Methode nicht zulässig |
| `BODY_TOO_LARGE` | 413 | Anfrageinhalt über 1 MiB; Bild-Uploads über 4 MiB Multipart-Body oder 3 MiB Datei |
| `URI_TOO_LONG` | 414 | Anfrageziel über 2048 Byte |
| `REQUEST_TIMEOUT` | 503 | Anfrage überschritt ihr Zeitlimit |
| `SERVICE_UNAVAILABLE` | 503 | Abhängiger Dienst nicht verfügbar |
| `INTERNAL_ERROR` | 500 | Unerwarteter Fehler oder Panic |

Neue Handler ordnen erwartete Fehler über `server.APIError` und `server.HandleError` zu. Unbekannte Fehler werden ohne ihren internen Text als `INTERNAL_ERROR` ausgegeben. Der Body wird durch `http.MaxBytesReader` begrenzt; Handler müssen Lesefehler an `server.HandleError` übergeben, damit auch Streaming-Bodies zuverlässig mit 413 beantwortet werden. Der Request-Kontext hat ein Limit von fünf Sekunden. Handler und Datenbankaufrufe müssen den Kontext beachten; ein bereits geschriebener Response kann nach Ablauf nicht ersetzt werden. Der HTTP-Server begrenzt Header auf 16 KiB und Lese- und Schreibzeiten auf zehn Sekunden.

Access-Logs werden als `slog`-JSON ausgegeben. Sie enthalten nur Zeitpunkt, Service/Umgebung, Ereignis, Request-ID, HTTP-Methode, normalisierte Route, Status, Dauer und gegebenenfalls Fehlercode. Normale 4xx-Antworten erscheinen als `INFO`, 5xx als `ERROR`. URL-Parameter, Header, Cookies, Bodies, SQL- und fremde Fehlertexte werden nicht protokolliert.

Mit einem mindestens 32 Zeichen langen `NORDWINDS_METRICS_TOKEN` ist `/internal/metrics` nur am internen API-Port und mit `Authorization: Bearer <Token>` abrufbar. Die Metriken umfassen HTTP-Statusklassen und -Dauer, DB-Readiness, GORM-Fehler und langsame Abfragen sowie simulierte Payment-Ereignisse und -Fehler. Caddy veröffentlicht diesen Pfad nicht. Sicherung und Rücksicherung sind in [NW-24](../docs/progress/NW-24.md) beschrieben.

## Kundenanmeldung

`POST /api/v1/auth/register` erstellt Kundenkonten, `POST /api/v1/auth/login` stellt eine neue Session aus, `GET /api/v1/auth/session` liefert den aktuellen Kunden und CSRF-Token, `POST /api/v1/auth/logout` widerruft die Session. `GET /api/v1/customer/me` ist ausschließlich für die Kundenrolle freigegeben. Der [OpenAPI-Vertrag](../docs/api/openapi.yaml) beschreibt Antworten und Fehlercodes. Die Sicherheitsentscheidung einschließlich Passwort-Reset-Entwurf steht in [ADR-004](../docs/adr/ADR-004-authentication.md).

Für HTTPS-Umgebungen muss `APP_PUBLIC_ORIGIN` auf den exakten öffentlichen Origin gesetzt sein, etwa `https://shop.example.invalid`; `APP_ENV` darf dort nicht `development` sein. Die API startet dann nur mit einer HTTPS-Origin. Das Session-Cookie ist `Secure`, `HttpOnly`, `SameSite=Strict` und ohne `Domain`. Ausschließlich bei `APP_ENV=development` erlaubt die API ein lokales HTTP-Cookie für die Entwicklung; dabei können `localhost` und `127.0.0.1` bei gleichem Port als lokale Origin-Aliase verwendet werden. Schreibzugriffe mit Session benötigen `X-CSRF-Token` aus der Login- oder Session-Antwort. Admin- und Mitarbeiterrechte werden nicht über die öffentliche Registrierung vergeben.

Der Authentifizierungs-Integrationstest benötigt `NORDWINDS_AUTH_TEST_URL` für eine **separate** zunächst leere PostgreSQL-Datenbank mit Namen `nordwinds_test_*`. Er prüft Registrierung, falsche Passwörter, CSRF, Session-Fixation und -Ablauf, Rate-Limit, Rollenwechsel und Redaction. Ohne die Variable wird nur dieser Test übersprungen. Bei einem parallelen Gesamtlauf benötigt jeder PostgreSQL-Integrationstest eine eigene Datenbank.

E-Mails laufen über eine Transactional Outbox (Migration 15, [ADR-011](../docs/adr/ADR-011-e-mail-benachrichtigungen.md)):

- Trigger reihen Registrierung, Kontoschließung, Bestellstatus und Erstattungen in derselben Transaktion ein; `identity.Service` meldet zusätzlich gesperrte Anmeldungen.
- Der Dispatcher in der API versendet alle 5 Sekunden per SMTP, wiederholt Fehler mit Backoff und löscht gespeicherte Adressen nach dem Versand.
- Lokal zeigt Mailpit die Nachrichten unter `http://127.0.0.1:8025`.
- Ohne `MAIL_TRANSPORT=smtp` bleibt der Versand aus, und die Einträge warten in der Outbox.
- In der Produktion verweigert die API den Start mit Testpostfach, Klartext-SMTP oder fehlenden Zugangsdaten.
- `GET /api/v1/admin/notifications` und `POST /api/v1/admin/notifications/{id}/retry` zeigen Zustände ohne Adressen und planen Fehlschläge neu ein.
- Die PostgreSQL-Tests verwenden `NORDWINDS_NOTIFICATION_TEST_URL` und `NORDWINDS_NOTIFICATION_FLOW_TEST_URL`.

Login- und Registrierungsversuche werden für 15 Minuten datenbankgestützt begrenzt. Im Compose-Stack setzt ausschließlich Caddy den Header `X-Nordwinds-Client-IP` auf die TCP-Quelladresse; die API berücksichtigt ihn nur mit `NORDWINDS_TRUST_PROXY_CLIENT_IP=true`. Bei direktem API-Betrieb muss die Option deaktiviert bleiben, bis ein entsprechend vertrauenswürdiger Proxy vorgeschaltet ist. Die HTTP-Oberfläche setzt pro HTML-Antwort einen neuen CSP-Nonce; die Prüfung dazu ist Teil des Smoke-Tests.

## Geschützte Katalogverwaltung

Die Admin-API unter `/api/v1/admin` bietet Kategorie-, Produkt- und Varianten-CRUD, Produkt-Kategorie-Zuordnungen sowie `GET /api/v1/admin/audit` für die letzten 100 Änderungen. Diese Katalogrouten erfordern die Rolle `admin`; POST, PUT und DELETE zusätzlich den Session-CSRF-Token. Die Bestell- und Versandrouten unter `/api/v1/admin/orders` erlauben auch die Rolle `employee`. Die Angular-Verwaltungsseite liegt unter `/admin`. Der [OpenAPI-Vertrag](../docs/api/openapi.yaml) enthält die einzelnen Routen und vollständigen Schreibfelder. Fehlende Datensätze liefern 404, ungültige Eingaben 400 und Unique-/FK-Konflikte 409.

Migration `0005_admin_audit` schreibt erfolgreiche Änderungen gemeinsam mit dem Katalogdatensatz in einer Transaktion. Für den PostgreSQL-HTTP-Test `NORDWINDS_ADMIN_TEST_URL` auf eine weitere zunächst leere Datenbank `nordwinds_test_*` setzen. Es wird kein Standard-Admin angelegt; die kontrollierte Rollenvergabe und die noch offenen Betriebsentscheidungen sind in [NW-12](../docs/progress/NW-12.md) beschrieben.

## Produktbilder und Warenkorb

Migration 6 speichert bis zu sechs bereinigte JPEG-Bilder je Produkt in PostgreSQL. Admin-Upload und -Löschen verlangen Session und CSRF; öffentliche Bildpfade liefern nur Bilder veröffentlichter oder archivierter Produkte. [ADR-005](../docs/adr/ADR-005-product-media.md) beschreibt Limits und Speicherentscheidung. Der echte HTTP-Test verwendet eine eigene leere Datenbank über `NORDWINDS_MEDIA_TEST_URL`.

Migration 7 ergänzt einen kontogebundenen Warenkorb. `GET /api/v1/cart`, `POST /api/v1/cart/items` und `PUT/DELETE /api/v1/cart/items/{variantId}` verlangen eine Customer-Session, Änderungen zusätzlich CSRF. Mengen und Preis-/Bestandsstatus stammen bei jeder Antwort aus der Datenbank; ein Warenkorb reserviert keinen Bestand. [ADR-006](../docs/adr/ADR-006-customer-cart.md) hält die Entscheidung gegen einen anonymen Warenkorb fest. Der PostgreSQL-HTTP-Test verwendet `NORDWINDS_CART_TEST_URL` auf einer weiteren separaten Testdatenbank.

## Preise und Bestellungen

Migration 8 ergänzt die Steuerklasse je Produkt (19 % oder 7 %), prozentuale Rabattcodes und den Code je Warenkorb. `GET /api/v1/cart` liefert die vollständige Preisaufstellung aus `internal/pricing`. [ADR-007](../docs/adr/ADR-007-pricing-policy.md) beschreibt Bruttopreise, Rundung und Versand. Der PostgreSQL-HTTP-Test verwendet `NORDWINDS_PRICING_TEST_URL`.

Migration 9 macht Bestellungen transaktional und unveränderlich. `POST /api/v1/orders` verlangt Customer-Session, CSRF und `Idempotency-Key`, rechnet den Warenkorb neu und legt Bestellung, Positionen und Statuseintrag in einer Transaktion an. `GET /api/v1/orders/{orderNumber}` liefert nur eigene Bestellungen. [ADR-008](../docs/adr/ADR-008-order-creation.md) beschreibt Idempotenz, Statusübergänge und Datenbankschutz. Der PostgreSQL-HTTP-Test verwendet `NORDWINDS_ORDERS_TEST_URL`. Bestellungen lassen sich wegen der Trigger nicht per `DELETE` entfernen; Testdatenbanken räumen mit `TRUNCATE` auf.

## Bestandsreservierung und simulierte Zahlungen

Beim Checkout reserviert die API den Bestand für 15 Minuten. Die Reservierung wird mit dem Bestellkopf und den Positions-Snapshots in derselben Transaktion angelegt. Ein periodischer Worker gibt abgelaufene Reservierungen frei. `POST /api/v1/orders/{orderNumber}/cancel` storniert eine eigene offene Bestellung und gibt deren Bestand frei; wiederholter Abbruch bleibt idempotent. Die Sperrreihenfolge und Wiederholung nach PostgreSQL-Deadlocks beschreibt [ADR-010](../docs/adr/ADR-010-bestandsreservierung.md).

Bei `APP_ENV=development` stehen die simulierten Zahlungsarten `debit_card`, `klarna`, `paypal` und `bank_transfer` zur Verfügung. Sie verarbeiten kein Geld und nehmen keine Karten- oder Bankdaten an. `GET /api/v1/mock-payments/methods` meldet die verfügbaren Testarten. `POST /api/v1/mock-payments` initiiert die Zahlung für eine eigene Bestellung; `POST /api/v1/mock-payments/{paymentId}/simulate` löst mit `succeeded`, `failed` oder nach Erfolg `refunded` ein signiertes Testereignis aus. Beide Schreibaufrufe benötigen Kunden-Session und `X-CSRF-Token`. `GET /api/v1/mock-payments/{paymentId}` oder `GET /api/v1/mock-payments/by-order/{orderId}` liest den Zustand einer eigenen Zahlung. [ADR-009](../docs/adr/ADR-009-simulierte-zahlungen.md) und [OpenAPI](../docs/api/openapi.yaml) beschreiben Zustandswechsel und Fehlerantworten.

Der Mock-Webhook unter `POST /api/v1/mock-payments/webhook` prüft `X-Nordwinds-Mock-Signature` über die unveränderten JSON-Bytes. Für externe Testskripte kann `NORDWINDS_MOCK_WEBHOOK_SECRET` in der lokalen `.env` auf mindestens 32 Zeichen gesetzt werden; ohne Wert erzeugt die API bei jedem Start einen neuen Schlüssel. Die Signatur hat das Format `t=<Unix-Sekunden>,v1=<HMAC-SHA256-Hex>` und deckt `timestamp + "." + body` ab. Doppelte Ereignis-IDs ändern den Zustand nicht erneut. In anderen `APP_ENV`-Umgebungen sind die Mock-Routen deaktiviert; sie ersetzen keinen echten Zahlungsdienst.

Die PostgreSQL-Tests für Reservierung und Mock-Zahlung benötigen jeweils eine **eigene**, zunächst leere Datenbank mit Namen `nordwinds_test_*` über `NORDWINDS_INVENTORY_TEST_URL` beziehungsweise `NORDWINDS_MOCK_PAYMENT_TEST_URL`. Sie prüfen unter anderem zwei gleichzeitige Zugriffe auf das letzte Stück, Ablauf, Stornierung, Deadlock-Retry, signierte Webhooks, Duplikate, Fehler und Erstattung. Ohne die Variablen werden nur diese Integrationstests übersprungen.

## Versand und Kundenkonto

Migration 12 ergänzt manuelle Sendungsnummern und Versandversuchszähler; Migration 13 ergänzt ein kundengebundenes Adressbuch. Migration 14 erlaubt die eingeschränkte Rolle `employee` und verknüpft manuelle Statusänderungen mit der handelnden Konto-ID. `GET /api/v1/orders` und `GET /api/v1/orders/{orderNumber}` zeigen ausschließlich eigene Bestellungen mit Statusverlauf und Sendung. Admins und Mitarbeiter verwenden `GET /api/v1/admin/orders`, `PATCH /api/v1/admin/orders/{orderNumber}/status` und `GET /api/v1/admin/orders/shipment-metrics`; Schreibzugriffe benötigen CSRF. Gültige Statusübergänge sind `PAID` → `PROCESSING` → `SHIPPED` → `DELIVERED`; für `SHIPPED` ist `tracking_number` erforderlich. Es gibt noch keine Anbindung an einen Versanddienst.

`GET/POST /api/v1/customer/addresses` sowie `PUT/DELETE /api/v1/customer/addresses/{id}` verwalten eigene deutsche Adressen. `GET /api/v1/customer/export` exportiert alle eigenen Adressen und Bestellungen. `DELETE /api/v1/customer/me` schließt ein Konto ohne offene Bestellungen, entfernt Sessions, Warenkorb und Adressbuch und ersetzt die Konto-E-Mail. Bestellsnapshots mit personenbezogenen Daten bleiben erhalten; Wirkung und offene Aufbewahrungsentscheidung stehen in [NW-20](../docs/progress/NW-20.md). Der PostgreSQL-HTTP-Test verwendet eine weitere separate, zunächst leere Datenbank über `NORDWINDS_SHIPPING_TEST_URL`.

## Beispielkatalog

`seeds/demo_catalog.sql` enthält 10 Kategorien mit je 2 veröffentlichten Beispielprodukten und einer aktiven Variante pro Produkt. Preise und Bestände sind erfundene Entwicklungsdaten; Produktbilder sind nicht enthalten. Das Skript ergänzt nur fehlende Datensätze, überschreibt bestehende Einträge nicht und bricht bei Slug-, SKU- oder ID-Kollisionen transaktional ab. Es wird bewusst nicht bei jeder Migration ausgeführt.

Im Repository-Stamm mit eingerichteter `.env` lässt sich der Beispielkatalog in die laufende lokale Compose-Datenbank laden:

```powershell
Get-Content -Raw -Encoding UTF8 backend/seeds/demo_catalog.sql |
  docker compose exec -T db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -X -v ON_ERROR_STOP=1'
```

Ein zweiter Lauf ist möglich und fügt keine Duplikate hinzu. Die öffentlichen Endpunkte `/api/v1/categories?limit=100` und `/api/v1/products?limit=100` zeigen danach die 10 bzw. 20 Datensätze.

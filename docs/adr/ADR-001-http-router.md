# ADR-001: HTTP-Router der Go-API

- **Status:** Angenommen am 29. Oktober 2026 durch den Projektinhaber: **Gin (`gin-gonic/gin`) bleibt das HTTP-Framework.**
- **Verlauf:** Am selben Tag wurde zunächst `net/http.ServeMux` vorgeschlagen und angenommen (Option B, Umbau als NW-04a). Noch vor jeder Umsetzung hat der Projektinhaber diese Entscheidung zurückgenommen und Gin festgelegt; die neue Fassung von `_todo/Golang.md` (S8b) schreibt Gin vor. Die Bewertung unten bleibt als Entscheidungsgrundlage erhalten. NW-04a entfällt.
- **Betrifft:** `backend/internal/server`, `backend/go.mod`; REQ-ARCH-01, REQ-ARCH-02, REQ-API-01 und REQ-SEC-01; NW-05 und alle folgenden HTTP-Schritte.
- **Grundlage:** [Dokumentenabgleich](../requirements/document-review.md) mit den Quellen S2, S5, S7, S8, S8b und dem Fahrplan.

## Kontext

Zum Zeitpunkt der Entscheidung hatte NW-03 für die minimale API Gin `v1.12.0` eingeführt. Der Router registrierte drei Routen (`/health/live`, `/health/ready`, `/api/v1/meta`) und nutzte `gin.New()`, `gin.Recovery()`, `SetTrustedProxies(nil)` und `c.JSON`. NW-05 setzte anschließend Problem-Antworten nach RFC 9457, Request-ID und strukturierte Logs auf Gin um; diese Änderungen wurden mit dem Katalog integriert.

Die Projektquellen beschreiben die HTTP-Schicht so:

| Quelle | Vorgabe |
| --- | --- |
| S8b §1, §3, §7 (`Golang.md`, Fassung 29.10.2026) | **Gin** als HTTP-Framework; Domain-Schicht unabhängig von Gin; Gin-Binding und -Validierung; `gin.Recovery()` kontrolliert einsetzen. |
| S7 §2 (`Angular.md`, gelesene Fassung) | `net/http` oder ein begründeter leichter Router; keine unnötigen Frameworks. |
| S8 §3 (`Golang.md`, gelesene Fassung) | `net/http`; jede zusätzliche Bibliothek begründen. |
| S2 §3 und `_todo/2_Logging_Error_Handling.md` (bis 29.10.) | `net/http` mit `chi` als dünner Routing-Schicht. |
| S5 §3 | `net/http`, bei Bedarf ein kleiner Router. |
| Fahrplan §1 (gelesene Fassung) | Chi oder der vorhandene minimale Router. |

Bis zur neuen Fassung S8b nannte keine Quelle Gin. S8b ist die jüngste ausdrückliche Vorgabe des Projektinhabers.

## Problem

Welche Routing-Schicht soll die API dauerhaft verwenden, nachdem NW-05 Middleware und Fehlerformat bereits darauf aufbaut?

## Entscheidungstreiber

1. Ausdrückliche Vorgabe des Projektinhabers (S8b).
2. Vorhandener Code: NW-03 und NW-05 nutzen Gin; ein Wechsel würde fertige Arbeit verwerfen.
3. Kleine Lieferkette und geringer Aufwand für Sicherheitsupdates (`govulncheck` in NW-04).
4. Volle Kontrolle über 404-, 405- und Fehlerantworten im Format RFC 9457.
5. Domain- und Service-Code bleibt testbar und frameworkunabhängig.

## Belegte Ausgangslage

- `backend/go.mod` (Stand `main`) enthält zwei direkte Module (Gin, pgx) und 33 indirekte Module. 29 der 33 indirekten Module stehen auch in der `go.mod` von Gin `v1.12.0`, darunter `bytedance/sonic`, `go-playground/validator/v10`, `go.mongodb.org/mongo-driver/v2`, `quic-go/quic-go`, `google.golang.org/protobuf`, `goccy/go-yaml` und `pelletier/go-toml/v2`. Die übrigen vier gehören zu pgx.
- Der Sicherheitsscan aus NW-04 fand sieben erreichbare Schwachstellen in `golang.org/x/net`, `golang.org/x/text` und `github.com/quic-go/quic-go`; nach den Updates meldete er null erreichbare Schwachstellen. `x/net` und `quic-go` sind direkte Anforderungen von Gin `v1.12.0`.
- In `gin.New()` ist `RedirectTrailingSlash` standardmäßig aktiv und `HandleMethodNotAllowed` deaktiviert; ohne Konfiguration wird eine falsche Methode mit 404 statt 405 beantwortet.
- Seit Go 1.22 unterstützt `net/http.ServeMux` Methoden- und Platzhaltermuster sowie 405 mit `Allow`-Header (Grundlage von Option B).

## Betrachtete Optionen

### A — Gin beibehalten

- **Vorteile:** vom Projektinhaber vorgegeben (S8b); NW-03 und NW-05 bereits umgesetzt und getestet; Route-Gruppen für öffentliche, Kunden- und Admin-Bereiche; Binding und Validierung integriert; weit verbreitet.
- **Nachteile:** größere Lieferkette (BSON-, Protobuf-, YAML-, TOML- und HTTP/3-Unterstützung); zwei der drei in NW-04 betroffenen Module stammen aus Gin. Handler hängen an `*gin.Context`. Standardeinstellungen für Slash-Weiterleitung, 405 und Proxy-Header müssen bewusst konfiguriert werden. Weicht von den früheren Fassungen S2, S5, S7 und S8 ab.

### B — `net/http.ServeMux` der Standardbibliothek

- **Vorteile:** keine zusätzliche Abhängigkeit; Methoden-Routing, 405 mit `Allow` und Konfliktprüfung eingebaut; Middleware nach `http.Handler`-Standard.
- **Nachteile:** keine Route-Gruppen; eigene Middleware- und JSON-Hilfen; die NW-05-Umsetzung müsste neu geschrieben werden; widerspricht S8b.

### C — `go-chi/chi/v5`

- **Vorteile:** `net/http`-kompatibel, ohne weitere Abhängigkeiten; Route-Gruppen, `Mount`, eigene `NotFound`-/`MethodNotAllowed`-Handler.
- **Nachteile:** zusätzliche Abhängigkeit; NW-05 müsste umgebaut werden; widerspricht S8b.

### Bewertung

| Kriterium | A: Gin | B: `net/http` | C: chi |
| --- | --- | --- | --- |
| Aktuelle Vorgabe S8b | erfüllt | nicht erfüllt | nicht erfüllt |
| Frühere Vorgaben S2/S5/S7/S8 | nicht erfüllt | erfüllt | erfüllt, mit Begründung |
| Zusätzliche Module | Gin und 29 der 33 indirekten | keine | eines |
| Handler-Typ | `*gin.Context` | `http.Handler` | `http.Handler` |
| 405 bei falscher Methode | nach Konfiguration | eingebaut, mit `Allow` | eingebaut, anpassbar |
| Route-Gruppen | ja | nein | ja |
| Aufwand nach NW-05 | keiner | mittel (Middleware neu) | mittel (Middleware neu) |

## Entscheidung

**Option A: Gin bleibt das HTTP-Framework der API.** Ausschlaggebend sind die ausdrückliche Vorgabe des Projektinhabers und der bereits darauf aufgebaute Code. Option B wird nicht umgesetzt; der geplante Schritt NW-04a entfällt.

## Auflagen für die Umsetzung

- **Gin nur in der HTTP-Schicht:** Domain-, Service- und Repository-Pakete importieren Gin nicht (S8b §3–4). Handler übersetzen zwischen `*gin.Context`, DTOs und Service-Aufrufen; Geschäftsregeln liegen nicht im Handler.
- **`gin.New()` statt `gin.Default()`:** eigene Recovery- und Logging-Middleware ohne interne Details in Antworten oder Logs (NW-05).
- **Proxy-Header:** `SetTrustedProxies(nil)` bleibt, bis eine Vertrauensregel für Caddy bewusst festgelegt wird (NW-23).
- **405 und Slash-Weiterleitung:** `HandleMethodNotAllowed` aktivieren und 404/405 als RFC 9457 beantworten; `RedirectTrailingSlash` bewusst festlegen und mit Tests belegen.
- **Binding und Validierung:** nur über explizite Request-DTOs, nie direkt auf Domain-Entities (Mass Assignment); Validierungsfehler werden auf das RFC-9457-Format abgebildet.
- **Betrieb:** `gin.SetMode(gin.ReleaseMode)` in Staging und Produktion.
- **Lieferkette:** `govulncheck` und Dependency-Updates bleiben Pflicht in CI; Gin-Release-Hinweise werden vor Updates geprüft.

## Folgen

**Positiv:** kein Umbau; NW-05 bleibt gültig; Route-Gruppen erleichtern RBAC für Kunden- und Admin-Bereiche; Binding und Validierung sind verfügbar.

**Negativ:** größere Lieferkette und mehr Sicherheitsupdates außerhalb der Go-Toolchain; Handler sind an Gin gebunden; mehrere Standardeinstellungen müssen bewusst konfiguriert und getestet werden.

## Risiken und Gegenmaßnahmen

| Risiko | Gegenmaßnahme |
| --- | --- |
| Erreichbare Schwachstellen in Gin-Abhängigkeiten | `govulncheck` in CI, zeitnahe Updates, Befund im PR dokumentieren |
| Kopplung von Geschäftslogik an Gin | Domain/Service ohne Gin-Import; Prüfung per `go list -deps` als Fitness Function |
| Unpassende Standardeinstellungen (405, Slash, Proxy) | Ausdrückliche Konfiguration und `httptest`-Tests |
| Mass Assignment über Binding | Explizite DTOs mit Allowlist-Feldern |

## Validierung

- CI: `gofmt`, `go vet ./...`, `go test ./...` und `govulncheck ./...` bestehen.
- Tests: `POST /health/live` liefert 405 als `application/problem+json`; unbekannte Pfade liefern 404 als Problem-Antwort; gefälschtes `X-Forwarded-For` beeinflusst die Client-IP nicht.
- `go list -deps` für Domain-, Service- und Repository-Pakete enthält `github.com/gin-gonic/gin` nicht.

## Wiedervorlage

- Wiederholt erreichbare Schwachstellen in Gin-Abhängigkeiten ohne zeitnahe Korrektur.
- Die Wartung von Gin stockt oder eine benötigte Funktion ist mit Gin unverhältnismäßig aufwendig.
- Dann Option B oder C erneut bewerten; die Analyse oben bleibt dafür gültig.

## Referenzen

- [Gin-Releases](https://github.com/gin-gonic/gin/releases)
- [`go.mod` von Gin 1.12.0 (Fossies-Archiv)](https://fossies.org/linux/gin/go.mod)
- [Go-Blog: Routing Enhancements for Go 1.22](https://go.dev/blog/routing-enhancements)
- [go-chi/chi](https://github.com/go-chi/chi)
- [RFC 9457: Problem Details for HTTP APIs](https://www.rfc-editor.org/rfc/rfc9457)

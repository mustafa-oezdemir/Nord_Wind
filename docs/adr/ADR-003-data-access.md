# ADR-003: Datenzugriff der Go-API (GORM)

- **Status:** Teilweise umgesetzt am 29. Oktober 2026: GORM-Verbindungsaufbau (NW-08a, Step 1) auf Nutzervorgabe. Die Umstellung der Repositories gemäß Option B bleibt vorgeschlagen.
- **Betrifft:** `backend/internal/database`, die Repositories in `backend/internal/*` (derzeit `catalog`), `backend/go.mod`; REQ-DATA-01, REQ-CAT-01 bis REQ-CAT-03, REQ-ORDER-01, REQ-INV-01; NW-07, NW-08 und alle folgenden Datenbankschritte.
- **Grundlage:** [Dokumentenabgleich](../requirements/document-review.md) mit den Quellen S2, S5, S8b und S8c; [ADR-002](ADR-002-domain-schema.md) für das Domain-Schema.

## Kontext

Stand vom 29. Oktober 2026 (Branch `codex/nw-08-catalog-api`): `database.OpenFromEnv` öffnet einen `*sql.DB` über den pgx-Treiber (`github.com/jackc/pgx/v5/stdlib`). Das Katalog-Repository aus NW-07 verwendet handgeschriebenes SQL und ist mit Integrationstests gegen echtes PostgreSQL abgesichert. Das Schema wird über versionierte SQL-Migrationen mit eigenem Runner (up/down) geführt. NW-08 (Katalog-API) ist in Arbeit und nutzt denselben Weg.

Die Projektquellen beschreiben den Datenzugriff so:

| Quelle | Vorgabe |
| --- | --- |
| S8c (`_todo/Golang.md`, GORM-Abschnitt) | GORM (`gorm.io/gorm`, `gorm.io/driver/postgres`) in den Repositories, zunächst mit der Traditional API. Versionierte SQL-Migrationen bleiben; `AutoMigrate` höchstens in der Entwicklung. Service-Schicht ohne GORM-Abhängigkeit. Transaktionen mit `db.Transaction` und `clause.Locking`, `TranslateError`, kein zweiter Verbindungspool. |
| S8b §3 (`_todo/Golang.md`, Gin-Abschnitt) | `pgx`/`pgxpool`; ein ORM ist nicht erforderlich, zunächst explizite SQL-Abfragen. |
| S2 §3 | `pgx`, bei Bedarf `sqlc`. |
| S5 §3 | `pgx`; bestehenden Code erhalten, `sqlc` nach Bedarf. |

S8b und S8c stehen in derselben Datei und widersprechen sich. S8c ist die jüngere Ergänzung des Projektinhabers.

## Problem

Soll die API weiterhin handgeschriebenes SQL über `database/sql` und pgx verwenden oder auf GORM umstellen – und wenn ja, zu welchem Zeitpunkt?

## Entscheidungstreiber

1. Vorgabe des Projektinhabers (S8c) gegenüber S8b, S2 und S5.
2. Korrektheit bei Geld, Bestand und Transaktionen: Sperren, `RowsAffected`, unveränderliche Snapshots.
3. Erhalt der versionierten SQL-Migrationen und der bestehenden Integrationstests.
4. Lieferkette und Wartungsaufwand.
5. Umbauaufwand: NW-07 und NW-08 sind bereits mit SQL umgesetzt.

## Betrachtete Optionen

### A — `database/sql` + pgx mit handgeschriebenem SQL beibehalten

- **Vorteile:** volle Kontrolle über SQL, Sperren und Indexnutzung; keine weitere Abhängigkeit; NW-07 und NW-08 bleiben unverändert.
- **Nachteile:** mehr Boilerplate für Scannen und Mapping; widerspricht S8c.

### B — GORM in den Repositories, SQL-Migrationen bleiben führend

- **Vorteile:** entspricht S8c; weniger Boilerplate für CRUD, Preload und Pagination; Transaktions-Helfer; Fehlerübersetzung über `TranslateError`.
- **Nachteile:** zusätzliche Abhängigkeiten. Bekannte Fallstricke: `Updates()` mit Struct übergeht Nullwerte (`false`, `0`, `""`), `Save()` kann ungewollt einfügen, Assoziationen werden implizit mitgespeichert, `Preload` kann N+1-Muster oder unnötige Last erzeugen. Generiertes SQL muss geprüft werden; Modell-Tags müssen zum Migrationsschema passen. NW-07 und NW-08 müssen umgestellt werden.

### C — sqlc (SQL-first mit generiertem, typsicherem Code)

- **Vorteile:** explizites SQL bleibt erhalten, weniger Boilerplate; in S2 und S5 genannt.
- **Nachteile:** Codegenerierung in Build und CI; widerspricht S8c; nicht angefragt.

### Bewertung

| Kriterium | A: SQL + pgx | B: GORM | C: sqlc |
| --- | --- | --- | --- |
| Aktuelle Vorgabe S8c | nicht erfüllt | erfüllt | nicht erfüllt |
| S8b / S2 / S5 | erfüllt | nicht erfüllt | erfüllt (S2, S5) |
| Kontrolle über SQL und Sperren | vollständig | über `clause`, `Raw`, Prüfung nötig | vollständig |
| Zusätzliche Abhängigkeiten | keine | `gorm.io/gorm`, `gorm.io/driver/postgres` | Generator im Build |
| Umbau von NW-07/NW-08 | keiner | ja | ja |
| Boilerplate | hoch | niedrig | mittel |

## Entscheidung (vorgeschlagen)

**Option B** wird für die Repository-Umstellung vorgeschlagen, weil der Projektinhaber GORM vorgibt (S8c). **NW-08a Step 1** hat nur GORM auf dem bestehenden Pool eingerichtet. Die laufende Katalog-API wurde dabei nicht umgebaut; die Integrationstests aus NW-07 und NW-08 dienen als Regressionsnetz. Bis zur Entscheidung über die Repository-Umstellung bleibt neuer Repository-Code bei SQL und pgx.

## Auflagen für NW-08a

- **Schema:** Die versionierten SQL-Migrationen bleiben die einzige Schemaquelle. `AutoMigrate` läuft weder im `migrate`-Job noch in Staging oder Produktion.
- **Verbindung:** GORM setzt auf den bestehenden `*sql.DB` auf (`postgres.New(postgres.Config{Conn: sqlDB})`); kein zweiter Pool, die Pool-Einstellungen bleiben in `internal/database`.
- **Fehler:** `gorm.Config{TranslateError: true}`; das Repository bildet `ErrRecordNotFound`, `ErrDuplicatedKey` und `ErrForeignKeyViolated` mit `errors.Is` auf Domain-Fehler ab. Keine Datenbankdetails in API-Antworten (RFC 9457, ADR-001).
- **Modelle:** kein `gorm.Model`; UUID-Schlüssel, Zeitstempel und Löschverhalten exakt wie im Schema (ADR-002); explizite Tags.
- **Updates:** explizite Felder über `Select` oder `map`; kein `Save()` für Teil-Updates; Assoziationen nicht automatisch speichern.
- **Geld und Steuer:** `int64` Cent; Steuersätze ganzzahlig (Basispunkte).
- **Bestand:** `db.Transaction` mit `clause.Locking{Strength: "UPDATE"}` oder bedingtem Update samt `RowsAffected`-Prüfung; stabile Sperrreihenfolge.
- **Schichten:** GORM nur im Repository. Service und Domain importieren `gorm.io/gorm` nicht; Gin-Handler rufen nie GORM auf.
- **Kontext und Logging:** jeder Aufruf mit `WithContext(ctx)`; GORM-Logger an `log/slog` angebunden, ohne SQL-Parameter mit personenbezogenen Daten; Schwelle für langsame Abfragen.

## Folgen

**Positiv:** entspricht der Vorgabe; weniger Mapping-Code; einheitliche Transaktions- und Fehlerbehandlung.

**Negativ:** Umbau des Katalog-Repositorys; zusätzliche Abhängigkeiten; generiertes SQL muss bei kritischen Pfaden (Bestand, Checkout) geprüft werden; Tags und Migrationen müssen synchron bleiben.

## Risiken und Gegenmaßnahmen

| Risiko | Gegenmaßnahme |
| --- | --- |
| Nullwerte werden bei `Updates()` übergangen | Tests für `false`, `0`, `""` und Bestand `0`; explizite Feldlisten |
| Modell weicht vom Migrationsschema ab | Integrationstests gegen migrierte Datenbank; kein `AutoMigrate` außerhalb lokaler Vergleiche |
| Fehlende oder falsche Sperren im Checkout | Parallele Integrationstests; bei Bedarf `Raw`/`Exec` mit geprüftem SQL |
| Lecks von Personendaten im SQL-Log | Parameter-Logging aus; Logger-Konfiguration getestet |
| Schwachstellen in neuen Abhängigkeiten | `govulncheck` in CI; Modulanzahl vorher und nachher dokumentiert |

## Validierung

- Die Integrationstests aus NW-07 und NW-08 bestehen unverändert.
- Neue Tests: Nullwert-Updates, Rollback, paralleler Bestandsabbau, Duplicate-Key → 409.
- `govulncheck ./...` meldet keine erreichbaren Schwachstellen.
- `go list -deps` der Service- und Domain-Pakete enthält `gorm.io/gorm` nicht.

## Wiedervorlage

- Messungen zeigen ineffiziente Abfragen oder falsche Sperren → für die betroffenen Repositories gezielt Raw SQL oder Option A/C.
- GORM-Updates führen zu wiederholten Regressionen bei kritischen Pfaden.

## Referenzen

- [GORM-Dokumentation](https://gorm.io/docs/): [Verbindung](https://gorm.io/docs/connecting_to_the_database.html), [Update](https://gorm.io/docs/update.html), [Transaktionen](https://gorm.io/docs/transactions.html), [Migration](https://gorm.io/docs/migration.html), [Fehlerbehandlung](https://gorm.io/docs/error_handling.html), [Kontext](https://gorm.io/docs/context.html)

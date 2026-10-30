# Übergabe an Claude — offene Aufgaben für NordWinds

**Stand:** 29. Oktober 2026. Vor jeder Fortsetzung Branch, Arbeitsbaum, PR-Basis und Testumgebung erneut prüfen. Diese Datei ist eine Aufgabenliste; ein lokaler Test ersetzt keinen GitHub-Actions-Lauf und ein Push ersetzt keinen Merge.

## 1. Aktuellen GORM-Schritt abschließen

Step 1 – GORM Setup wurde über [PR #10](https://github.com/mustafa-oezdemir/Nord_Wind/pull/10) in `main` integriert. Der implementierte Stand und die wirklich ausgeführten Prüfungen sind in [NW-08a](../progress/NW-08a.md) dokumentiert.

- [ ] Den integrierten Step-1-Diff und die Integrationsentscheidung prüfen. `gorm.io/gorm v1.31.2` und `gorm.io/driver/postgres v1.6.3` sind eingebunden; API und GORM teilen einen `database/sql`/pgx-Pool. Migrationen und Katalog-Repositories nutzen weiterhin SQL. Keine produktive Datenbank migrieren und `AutoMigrate` nicht einschalten.
- [ ] `go test -race ./...` mit getrennten PostgreSQL-Testdatenbanken in einer CGO-fähigen Umgebung nachholen. Der normale Gesamtlauf mit vier Datenbanken, `go vet`, Go-/Docker-Build und ein isolierter Docker-Smoke-Test waren erfolgreich.
- [x] `govulncheck@v1.9.0 ./...` lief auf dem integrierten Stand in einem Linux-Container: null erreichbare Schwachstellen, drei Meldungen in benötigten Modulen ohne Aufruf durch den Code. Ein grüner GitHub-Actions-Lauf steht weiterhin aus.
- [ ] Den GORM-Startpfad und `main.go` auf Schließung des Pools bei allen frühen Fehlerpfaden prüfen; gegebenenfalls `run() error` statt `os.Exit` innerhalb der Ressourcenlebensdauer verwenden. Den Redaction-Test für den `slog`-Adapter erhalten.
- [ ] Vor dem nächsten GORM-Schritt die Traditional API gegen die Generics API am tatsächlichen Repository abwägen. [ADR-003](../adr/ADR-003-data-access.md) beschreibt die vorgeschlagene Repository-Umstellung; Step 1 entscheidet diese Umstellung noch nicht.

## 2. GORM-Auftrag nach Schritt 1

Die folgenden Punkte sind **nicht** Bestandteil der Step-1-Implementierung. Die bestehende REST-API, JSON-Verträge und SQL-Migrationen müssen während des Umbaus erhalten bleiben.

- [ ] **Step 2 – Model Mapping:** Nur vorhandene Tabellen abbilden: `categories`, `products`, `product_categories`, `product_variants`, `customers`, `orders`, `order_items`. UUID, FK, Unique-/Check-Regeln, `ON DELETE RESTRICT`, Zeitstempel und Cent-Beträge mit Migration 2 vergleichen. Kein `gorm.Model`, kein `AutoMigrate`, keine erfundenen `ProductImage`-, `User`-, `Address`- oder `Cart`-Tabellen.
- [ ] **Step 3 – Repository Integration:** Das vorhandene `internal/catalog`-Repository schrittweise auf GORM umstellen. Context, parametrisierte Bedingungen, `ErrNotFound`, Duplicate-/FK-Fehler und unveränderte HTTP-Antworten nachweisen. Zero-Value-Updates für `false`, `0`, leeren Text und Bestand `0` mit echten PostgreSQL-Tests absichern. Keine impliziten `Save()`-Upserts.
- [ ] **Step 4 – Relationships & Queries:** Kategoriebaum, M:N-Produktkategorien und Varianten korrekt laden; `Preload` nur für tatsächlich benötigte Beziehungen verwenden. Listenfilter, Suche, Sortier-Allowlist, Count und Pagination aus NW-08 exakt erhalten; N+1 und SQL-Pläne gegen Migration 3 prüfen.
- [ ] **Step 5 – Transactions:** Bestehende testbare Mehrschrittoperationen mit derselben GORM-Transaktion ausführen. Checkout, Warenkorb und Bestandsabbau existieren noch nicht; deren Sperren, Rollback und parallele Bestellungen erst in den vorgesehenen NW-14/NW-16/NW-17-Schritten implementieren.
- [ ] **Step 6 – Error Handling & Testing:** Echte PostgreSQL-Regressionen für CRUD, Beziehungen, Rollback, Fehlerübersetzung und HTTP-Vertrag ausführen. `go test -race ./...`, `go vet ./...`, Docker-Smoke, `govulncheck`, Log-Redaction und Dependency-Diff prüfen. Fehlende Prüfungen als offen kennzeichnen.

## 3. Integrierte PRs und verbleibende CI-Prüfung

- [x] [PR #7 — NW-06 Domain](https://github.com/mustafa-oezdemir/Nord_Wind/pull/7) und [PR #10 — NW-08a GORM Setup](https://github.com/mustafa-oezdemir/Nord_Wind/pull/10) sind in `main`. Die Commits von [PR #8 — NW-07 Repository](https://github.com/mustafa-oezdemir/Nord_Wind/pull/8) und [PR #9 — NW-08 Katalog-API](https://github.com/mustafa-oezdemir/Nord_Wind/pull/9) wurden als Vorfahren von PR #10 ebenfalls integriert.
- [x] Die Änderungen aus [PR #5 — NW-04 CI](https://github.com/mustafa-oezdemir/Nord_Wind/pull/5) und [PR #6 — NW-05 API-Fehler](https://github.com/mustafa-oezdemir/Nord_Wind/pull/6) wurden auf einem gemeinsamen Integrationsbranch mit dem Katalog und GORM zusammengeführt. Der Katalog verwendet nun das gemeinsame Problemformat mit `request_id`; `httptest` und Go-Gesamttests bestanden lokal.
- [ ] GitHub Actions `quality` erneut ausführen. [PR #11](https://github.com/mustafa-oezdemir/Nord_Wind/pull/11) wurde in `main` integriert; der PR- und der anschließende `main`-Lauf endeten erneut vor jedem Runner-Schritt. Erst ein grüner Lauf belegt das CI-DoD; die Branch-Protection ist im privaten Repository derzeit nicht einrichtbar.

## 4. Weitere Roadmap-Aufgaben

Die Reihenfolge bleibt abhängig von den vorherigen Schritten. Für jede Aufgabe eigene Akzeptanztests, Fortschrittsnotiz, Commit und Push führen.

- [x] **NW-09:** Responsive Angular-Startseite mit echter Katalog-API ist in `main` ([Nachweis](../progress/NW-09.md)).
- [ ] **NW-10:** Produktdetail, Varianten, Preis-/Bestandsanzeige und fehlende Slugs.
- [x] **NW-11–12:** Identität, Sessions und Admin-Katalogpflege sind implementiert ([NW-11](../progress/NW-11.md), [NW-12](../progress/NW-12.md)). Der Passwort-Reset ist nur entworfen; produktive Rollenvergabe ist noch zu entscheiden.
- [ ] **NW-13–14:** Produktbilder und Mediensicherheit; Warenkorb. Fehlende Tabellen nur mit versionierten, datenbewahrenden Migrationen ergänzen.
- [ ] **NW-15–20:** Preis-, Steuer- und Versandvertrag fachlich entscheiden; Checkout-Transaktion; konkurrierenden Bestand; Payment-Webhooks; Versand/Tracking; Kundenkonto und Bestellhistorie.
- [ ] **NW-21–26:** Deutschland-spezifische E-Commerce-Pflichten; Accessibility/SEO; Security-Hardening; Observability/Backup/Restore; Staging-/Release-Pipeline; Produktionsstart und Monitoring. Rechtliche, Zahlungs- und Betriebsentscheidungen vor der Implementierung mit dem Projektinhaber klären.

## 5. Arbeitsbaum und Datensicherheit

Die fünf zuvor nur im lokalen Arbeitsbaum liegenden Dokumente (`docs/progress/NW-00.md`, `docs/requirements/backlog.md`, `docs/requirements/document-review.md`, ADR-001 und ADR-003) wurden für die Integration einzeln übernommen. Ihre ursprünglichen Dateien wurden nicht verändert. Der bestehende Compose-Stack und sein persistentes Datenbank-Volume wurden für die GORM-Prüfung nicht verändert. Für Integrations- und Migrationstests immer separate `nordwinds_test_*`-Datenbanken verwenden.

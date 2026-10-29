# ADR-002: Domain-Schema und Datenhoheit für Katalog und Bestellungen

- **Status:** Vorgeschlagen
- **Datum:** 29. Oktober 2026
- **Entscheidungsträger:** Projektinhaber; fachliche Produkt- und Preisentscheidungen noch offen
- **Bereich:** `backend/internal/catalog`, `identity`, `orders`, PostgreSQL-Migration 2
- **Anforderungen:** `REQ-CAT-01`, `REQ-CAT-02`, `REQ-MONEY-01`, `REQ-OPS-01`

## Kontext und Problem

Der verifizierte Stand aus NW-03 hat nur `app_settings` in Migration 1. Der lokale Compose-Stack besitzt ein persistentes Datenbank-Volume. NW-06 benötigt eine relationale Grundlage für Kategoriebaum, Produkte, verkaufbare Varianten und unveränderliche Bestell-Snapshots, ohne bestehende Datensätze zu löschen. Produktarten, Steuer- und Versandpolitik sowie Gast-Checkout sind noch nicht fachlich entschieden.

## Optionen und Entscheidungstreiber

| Option | Vorteil | Nachteil |
| --- | --- | --- |
| Preis, SKU und Bestand direkt am Produkt; eine Kategorie je Produkt | Wenige Tabellen | Varianten und Mehrfachkategorien müssten später mit Datenmigration nachgerüstet werden. |
| Produkt als redaktionelle Einheit; SKU, Cent-Preis und Bestand je Variante; M:N-Kategorien | Passt zum geplanten Katalog und zu Bestell-Snapshots | Mehr Beziehungen und Joins. |

**Vorgeschlagene Entscheidung:** Die zweite Option wird in Migration 2 umgesetzt. `categories`, `products`, `product_variants`, `customers`, `orders` und `order_items` verwenden UUID-Primärschlüssel mit PostgreSQL `gen_random_uuid()`; `product_categories` hat einen zusammengesetzten Primärschlüssel. Das minimale `customers`-Schema liefert nur den FK-Anker für Bestellungen; Authentifizierung und Sessions folgen in NW-11. EUR-Beträge liegen als `BIGINT`-Cent an Varianten, Bestellungen und Positionen. Go-Modelle verwenden dafür `int64`, für `INTEGER`-Mengen `int32` und für UUIDs vorerst `string`. Eine Produktart oder variantenabhängige Attributstruktur wird noch nicht erzwungen.

| PostgreSQL | Go-Modell | Zuordnung und Besitz |
| --- | --- | --- |
| `categories` | `catalog.Category` | `parent_id` → `*string`; Slug, Status und Reihenfolge im Catalog-Modul. |
| `products` | `catalog.Product` | Redaktioneller Produkttext und Veröffentlichungsstatus im Catalog-Modul. |
| `product_categories` | `catalog.ProductCategory` | Zusammengesetzter Schlüssel für die M:N-Beziehung. |
| `product_variants` | `catalog.ProductVariant` | Eindeutige SKU, `price_cents` → `int64`, `stock_quantity` → `int32`. |
| `customers` | `identity.Customer` | Minimaler Besteller-FK; Identitätsregeln folgen separat. |
| `orders` | `orders.Order` | Besteller, Status, `total_cents` → `int64` und Versand-Snapshot. |
| `order_items` | `orders.OrderItem` | Variant-FK und historische Namens-, SKU-, Preis- und Steuerwerte. |

## Datenregeln und Grenzen

- Kategorie- und Produkt-Slugs, SKU und E-Mail sind ohne Beachtung der Großschreibung eindeutig. FK-Beziehungen und `ON DELETE RESTRICT` schützen referenzierte Katalog- und Bestelldaten.
- Ein Trigger verhindert Kategoriezyklen; eine Transaktionssperre serialisiert Änderungen an `parent_id`. Nichtnegative Preise, Summen und Bestände, positive Positionsmengen, gültige Statuswerte und `EUR` werden durch benannte Check-Constraints geprüft.
- `order_items` speichert Name, SKU, Stückpreis und Steuerdaten als Snapshot. Änderungen am späteren Katalogpreis verändern diese Spalten nicht. Der genaue Steuerinhalt, Rundung, Statusübergänge, Order-Transaktion und technische Unveränderlichkeit nach dem Kauf werden in NW-15–NW-18 festgelegt und getestet. Solange gibt es keinen öffentlichen Checkout-Endpunkt.
- Ein Produkt kann im Schema zunächst ohne Kategorie oder Variante entstehen. Die Regel „veröffentlicht und kaufbar“ muss beim späteren Admin-/Catalog-Service geprüft werden. Produktbilder und Upload-Sicherheit folgen in NW-13.

## Migration, Rückweg und Datenverlustschutz

Migration 2 fügt nur neue Tabellen, Trigger, Constraints und Indizes hinzu; Migration 1 und `app_settings` bleiben erhalten. `Up` arbeitet mit Transaktion und Advisory Lock, erkennt Lücken und unbekannte Versionen. `Down` nimmt genau eine Version zurück, sperrt die betroffenen Tabellen gegen gleichzeitige Schreibvorgänge und scheitert innerhalb derselben Transaktion, wenn eine Tabelle Daten enthält. Für Migration 1 gilt dieselbe Sperre bei `app_settings`. Bei echten Bestell- oder Katalogdaten erfolgt eine Korrektur durch eine neue vorwärtsgerichtete Migration statt durch Löschen. Ein Datenexport und Restore-Plan sind vor späteren strukturellen Änderungen nötig.

## Folgen, Validierung und Wiedervorlage

Die Normalisierung erleichtert SKU- und Bestandsregeln, erhöht aber die Zahl der Joins. UUIDs sind ohne separate Sequenzvergabe erzeugbar; [PostgreSQL 18 dokumentiert `gen_random_uuid()`](https://www.postgresql.org/docs/18/functions-uuid.html). Unique- und FK-Indizes folgen den [PostgreSQL-Constraint-Regeln](https://www.postgresql.org/docs/18/ddl-constraints.html); zusätzliche Indizes decken Filter- und Beziehungsspalten ab. Die reale PostgreSQL-Integration prüft `up/down/up`, eine Version-1-Datenübernahme, Datenverlustsperren und ungültige SKU-, Preis-, Bestands- und FK-Werte. Diese Entscheidung wird vor NW-10/NW-12 für konkrete Produktarten und vor NW-15 für Steuer- und Preisregeln erneut geprüft.

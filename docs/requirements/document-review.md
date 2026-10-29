# NW-00: Abgleich der Projektdokumente

Stand: 29. Oktober 2026. Dieser Abgleich beschreibt Anforderungen und Entscheidungen, keine bereits implementierten Funktionen. Die Originale in `_todo/` bleiben lokal und unverändert.

## Quellen und Lesestatus

Alle sechs vorhandenen Master-Dokumente wurden inhaltlich geprüft. Die SHA-256-Präfixe dienen dazu, spätere Änderungen an den lokalen Quellen zu erkennen.

| Kürzel | Tatsächlicher Dateiname in `_todo/` | Relevante Abschnitte | SHA-256-Präfix | Status |
| --- | --- | --- | --- | --- |
| S1 | `NORDwind_MASTER_PROMPT.md` | 2–12: Markt, Stack, UI, Daten, Checkout, API, Sicherheit, Betrieb | `7E1C0124BDD6` | gelesen |
| S2 | `MASTER_PROMPT_ECOMMERCE_DE.md` | 2–17: Produkt, Architektur, API, Daten, Logging, Betrieb, Qualität | `E3309724D3DF` | gelesen |
| S3 | `Ecommerce_Deutschland_Master_Prompt.md` | 2–21: priorisierte Anforderungen, Zahlung, Risiken, Testfälle | `0E2EB8BAADA` | gelesen |
| S4 | `ecommerce_master_prompt.md` | 2–24: offene Fragen, Domäne, Architektur, API, Entwicklung | `A21BF64F74A` | gelesen |
| S5 | `MarktHaus_ECommerce_Master_Prompt.md` | 2–15: MVP, Architektur, Katalog, Sicherheit, Meilensteine | `0C04409055E7` | gelesen |
| S6 | `Docker.md` | 2–10: Stack, Container, Netzwerk, TLS, Sicherheit, Tests | `CE19E78EB652` | gelesen |

`NORDWINDS_DEVELOPMENT_ROADMAP.md` (SHA-256-Präfix `B438C91D6B85`) steuert die Reihenfolge NW-00 bis NW-26. Der Fahrplan nennt teilweise `todo/`; das vorhandene Verzeichnis heißt `_todo/`. Der dort genannte MarktHaus-Dateiname unterscheidet sich nur in der Großschreibung von S5. Die früheren Quelldateien `Promt.md`, Spezialisten-Dokumente und PDFs sind hier nicht vorhanden und wurden nicht als gelesen gewertet.

## Vorrang und tatsächlicher Projektstand

Die ausdrückliche Nutzervorgabe und die lokale `AGENTS.md` legen **Kuzey Rüzgarları** als Markennamen und `kuzey-ruzgarlari` als technischen Bezeichner fest. Ältere Namen in den Entwürfen sind historische Vorschläge. Öffentliche Projektdokumentation wird auf Deutsch geschrieben.

Das Repository enthält derzeit `README.md`, `.env.example`, ein mehrstufiges `Dockerfile`, `.dockerignore` und `ops/Caddyfile`. `frontend/`, `backend/`, `compose.yaml`, Datenbankmigrationen, OpenAPI und Tests fehlen. Aussagen in S5 über einen vorhandenen MarktHaus-Starter gelten daher **nicht** als Nachweis für dieses Repository. Die Docker-Ziele wurden geparst und Caddy validiert; ein vollständiger Anwendungs-Build wurde mangels Quellcode nicht ausgeführt.

## Zusammengeführte Anforderungen

Status bedeutet: **bestätigt** durch die aktuelle Nutzervorgabe oder eine übereinstimmende Projektentscheidung; **vorgeschlagen** für die Umsetzung, aber noch nicht durch Code belegt; **offen** bei fachlicher oder technischer Entscheidung; **später** für einen späteren Schritt.

| ID | Anforderung | Nachweis in den Quellen | Status | Umsetzungsschritt |
| --- | --- | --- | --- | --- |
| REQ-PROJ-01 | Einzelanbieter-Shop für Deutschland, Oberfläche `de-DE`, Währung `EUR`. | S1 §2; S2 §1–2; S3 §2; S4 §2; S5 §2; S6 §1 | bestätigt | durchgehend |
| REQ-BRAND-01 | Öffentliche Marke **Kuzey Rüzgarları**, technischer Name `kuzey-ruzgarlari`. | aktuelle Nutzervorgabe; S1 §2, S5 §8 und Fahrplan §1 enthalten ältere Namen | bestätigt | NW-02 und neue Artefakte |
| REQ-ARCH-01 | Angular/Bootstrap, Go-API, PostgreSQL und Caddy/Docker; Go als modularer Monolith. | S1 §3–4; S2 §3–4; S3 §4; S4 §8–10; S5 §3–4; S6 §2, §6 | vorgeschlagen | NW-01–04 |
| REQ-CAT-01 | Hierarchische Kategorien; Produkte können mehreren Kategorien zugeordnet sein. | S1 §5–6; S2 §2, §7; S3 §3; S4 §4–5; S5 §5–6; S6 §5 | vorgeschlagen | NW-06–09 |
| REQ-CAT-02 | Kaufbare Varianten haben eindeutige SKU, Preis und Bestandsbezug. | S4 §5, §11; S5 §5–6; Fahrplan §3 | vorgeschlagen | NW-06, NW-10 |
| REQ-CAT-03 | Katalogsuche, Filter, Sortierung und begrenzte Seitennummerierung. | S1 §8; S2 §5–6; S3 §3; S4 §4, §12; S5 §7 | vorgeschlagen | NW-08 |
| REQ-UI-01 | Responsive, zugängliche deutsche Oberfläche mit Lade-, Leer- und Fehlerzuständen; EUR-Formatierung. | S1 §5; S2 §5; S3 §8, §12; S4 §6; S5 §8; S6 §3 | vorgeschlagen | NW-09–10, NW-22 |
| REQ-API-01 | Versionierte JSON-API unter `/api/v1` mit OpenAPI, Validierung und begrenzten Antworten. | S1 §8; S2 §6, §9; S3 §13; S4 §12; S5 §7; S6 §4 | vorgeschlagen | NW-05, NW-08 |
| REQ-CART-01 | Warenkorb speichert Varianten-ID und positive Menge; Preise werden serverseitig erneut geprüft. | S1 §7; S2 §2, §8; S3 §3, §5; S4 §4–5; S5 §6; S6 §4 | vorgeschlagen | NW-14 |
| REQ-AUTH-01 | Sichere Anmeldung und Sessions, CSRF-Schutz bei Cookie-Authentifizierung sowie serverseitige Rollen- und Besitzprüfung. | S1 §9; S2 §10; S3 §14; S4 §14; S5 §9; S6 §7 | vorgeschlagen | NW-11–12 |
| REQ-ORDER-01 | Preise, Steuer und Versand werden serverseitig berechnet; Bestellung und unveränderliche Positions-Snapshots entstehen konsistent in einer Transaktion. | S1 §6–7; S2 §7–8; S3 §5–7; S4 §5, §11, §13; S5 §5–6; S6 §5 | vorgeschlagen | NW-15–16 |
| REQ-MONEY-01 | EUR-Beträge als ganzzahlige Cent oder begründete exakte Dezimalwerte; keine Float-Berechnung für Geld. | S1 §6; S2 §7; S3 §5; S4 §5; S5 §5; S6 §5 | vorgeschlagen | NW-15 |
| REQ-INV-01 | Atomare Bestandsreservierung verhindert Überverkauf auch bei parallelen Checkouts. | S1 §7; S2 §8; S3 §5–7; S4 §5, §11; S5 §6; S6 §4 | vorgeschlagen | NW-17 |
| REQ-PAY-01 | Zahlungen über einen externen Anbieter; signierte Webhooks, Idempotenz, Abgleich und keine Kartenspeicherung. | S1 §7, §9–10; S2 §8, §10; S3 §3, §6–7; S4 §5, §13; S5 §6, §9; S6 §7 | offen: Anbieter und Geschäftspolitik | NW-18 |
| REQ-ADMIN-01 | Geschützte Verwaltungsfunktionen für Katalog, Bestand und Bestellstatus mit Audit-Spur. | S1 §5, §8–9; S2 §2, §6, §10; S3 §3; S4 §4; S5 §7, §9; S6 §3 | vorgeschlagen | NW-12 |
| REQ-SEC-01 | Geheimnisse und Kundendaten bleiben aus Git, Images, Browser-Bundles und Protokollen; Datenbank bleibt nicht öffentlich. | S1 §4, §9; S2 §10, §14; S3 §14–15; S4 §14; S5 §9; S6 §6–7 | bestätigt als Sicherheitsgrenze | durchgehend |
| REQ-OPS-01 | Reproduzierbare Builds, kontrollierte Migrationen, Health/Readiness, Backups und geprüfte Wiederherstellung. | S1 §11–12; S2 §14, §16; S3 §18–20; S4 §19, §24; S5 §12, §15; S6 §6, §10 | vorgeschlagen | NW-01–04, NW-24–25 |
| REQ-OBS-01 | Strukturierte, datensparsame Logs mit Request-ID und technischen Fehlerklassen. | S1 §12; S2 §12–13; S3 §15; S4 §17; S5 §10; S6 §10 | vorgeschlagen | NW-05, NW-24 |
| REQ-TEST-01 | Risikoabhängige Unit-, API-, PostgreSQL-Integrations-, Komponenten- und ausgewählte E2E-Tests; CI berichtet echte Ergebnisse. | S1 §12, §15; S2 §16; S3 §9–11, §19; S4 §18; S5 §11; S6 §10 | vorgeschlagen | NW-01, NW-04 und jede Funktion |
| REQ-DE-01 | Preisangaben, Checkout, Widerruf, Datenschutz, Produktsicherheit und Barrierefreiheit werden für den tatsächlichen Verkauf rechtlich geprüft. | S1 §10; S2 §11; S3 §8; S4 §15; S5 §13; S6 §8 | offen: Produkt- und Geschäftsdetails | NW-21–22 |

## Abweichungen und Entscheidungen

| Thema | Abweichung | Behandlung |
| --- | --- | --- |
| Marke | Fahrplan: NordWinds; S1: Nordmarkt; S5: MarktHaus; S6: EuroShop. | Aktuelle Nutzervorgabe **Kuzey Rüzgarları** hat Vorrang. Historische Quelldateien bleiben unverändert. |
| Starter | S5 §14/§17 beschreibt vorhandenen Angular-/Go-/PostgreSQL-Code; im aktuellen Repository fehlt dieser. | NW-01 untersucht den tatsächlichen Stand und muss Bootstrap statt Starter-Audit planen. Kein vorhandenes Feature wird behauptet. |
| Zahlungszeitpunkt | S3 §2–3 fordert Onlinezahlung in der ersten verkaufsfähigen Version; S1 §13 und S5 §15 staffeln Katalog, Bestellung und Zahlung. | Technische Zwischenstände dürfen ohne Zahlung entstehen, aber nicht als vollständiger verkaufsfähiger Shop gelten. Umsetzung erst nach Preis-, Bestands- und Sessionregeln. |
| Gast-Checkout | S1 §6/§13 nennt Gast/Demo, S3 §2 und S4 §3 lassen Gast-Checkout offen, Fahrplan NW-14 setzt zunächst Sessions voraus. | Fachentscheidung vor Warenkorb-/Checkout-Vertrag erforderlich; kein öffentlicher ungeschützter Bestell-Endpunkt. |
| Produktpreis und Bestand | S1 §6 und S6 §5 legen Werte am Produkt nahe; S4 §11 und S5 §5 trennen Variante/Preis/Bestand. | Variante als Verkaufseinheit vorgemerkt; exakte Tabellen erst in NW-06 nach Daten- und Abfrageentwurf festlegen. |
| API-Pfade | Produktdetail wird mit `{id}` oder `{slug}` beschrieben; Health-Pfade variieren zwischen `healthz`, `health` und `/health/live`. | OpenAPI-Vertrag vor Implementierung festlegen; bis dahin keine Route als bestehend ausgeben. |
| Interne Verschlüsselung | S6 §7 fordert mTLS zwischen Caddy/API und TLS-Clientzertifikate zur DB; andere Quellen fordern private Netze und HTTPS am öffentlichen Eingang. | Verpflichtung/Abweichung offen dokumentiert. Lokales HTTP im jetzigen Caddyfile ist nur Entwicklung, keine Produktionsfreigabe. Sicherheitsentscheidung vor realem Betrieb. |
| SSR und Monitoring-Stack | S2 §3/§14 sieht SSR sowie Alloy/Loki/Prometheus/Grafana früh vor; S4 §6/§17 und S5 §4/§10 verlangen Bedarfsprüfung. | Statischer Angular-Build und `slog` zuerst; SSR sowie zentrale Telemetrie nach messbarem Bedarf und ADR entscheiden. |
| Dokumentationssprache | Mehrere Entwürfe wünschen türkische technische Erklärungen. | Aktuelle Nutzervorgabe: veröffentlichte Dokumentation auf Deutsch; lokale `_todo`-Originale bleiben unverändert. |

## Noch offene Geschäftsentscheidungen

Vor den jeweiligen Implementierungsschritten zu klären: physische Produktarten und Varianten, Gast-Checkout, Brutto-/Netto- und Rundungsregeln, Umsatzsteuerklassen, Versand- und Rückgaberegeln, Zahlungsanbieter, Reservierungsdauer, Hosting/Last/SLA sowie die fachliche und rechtliche Prüfung für Deutschland. Diese Punkte blockieren NW-01 nicht; sie blockieren eine echte Checkout- oder Produktionsfreigabe.

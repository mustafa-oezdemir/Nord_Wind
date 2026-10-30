# ADR-007: Preis-, Steuer-, Rabatt- und Versandregeln

- **Status:** Angenommen am 30. Oktober 2026 (NW-15). Die vier fachlichen Vorgaben (Preisbasis, Steuersätze, Versand, Rabatt) hat der Projektinhaber am selben Tag festgelegt; Rundung und Verteilung sind technische Entscheidungen dieses ADR.
- **Betrifft:** `backend/internal/pricing`, `backend/internal/cart`, Migration 8 (`products.tax_class`, `discount_codes`, `cart_discounts`), Admin-API, Warenkorb- und Produktseiten; REQ-MONEY-01, REQ-ORDER-01, REQ-DE-01.
- **Vorbehalt:** Es handelt sich um eine technische Umsetzung, keine Steuerberatung. Vor dem ersten Verkauf (NW-21) prüft eine steuerkundige Person die Steuerklassen der Produkte, die Aufteilung der Versandkosten und die Rechnungsangaben.

## Kontext

Bis NW-14 speicherte jede Variante einen Preis in Cent, ohne festzulegen, ob er brutto oder netto ist. Der Warenkorb zeigte nur eine Zwischensumme; Umsatzsteuer, Versand und Rabatte fehlten. Der Checkout (NW-16) braucht eine eindeutige, testbare Regel, damit API, Bestellung und Oberfläche dieselbe Summe zeigen.

Rechtslage, Stand 2026 (§ 12 UStG): Regelsatz 19 %, ermäßigter Satz 7 % für Waren der Anlage 2 (z. B. Bücher, die meisten Lebensmittel), Nullsatz nur für bestimmte Photovoltaikanlagen. Gegenüber Verbrauchern sind Gesamtpreise inklusive Umsatzsteuer anzugeben. Versandkosten einer Warenlieferung teilen als Nebenleistung den Steuersatz der Ware; bei gemischten Sätzen ist das Versandentgelt sachgerecht aufzuteilen.

## Entscheidungen

| Thema | Regel |
| --- | --- |
| Preisbasis | `product_variants.price_cents` ist der **Bruttopreis** in Euro-Cent. Die Oberfläche zeigt ihn unverändert mit „inkl. MwSt.“. |
| Steuerklasse | Je Produkt `tax_class` = `standard` (19 %) oder `reduced` (7 %); Standard ist `standard`. Admins wählen die Klasse; die Sätze stehen im Code (`pricing.taxRates`), nicht in der Datenbank. |
| Positionen | Positionssumme = Bruttostückpreis × Menge, exakt in Cent. Nicht lieferbare Positionen zählen nicht. |
| Rabatt | Ein prozentualer Code je Warenkorb (`percent_bp`, 1–10 000 Basispunkte). Rabatt = Warenwert × Prozentsatz, kaufmännisch auf Cent gerundet (ab 0,5 Cent aufrunden). Er wird im Verhältnis der Bruttowarenwerte auf die Steuersätze verteilt (Verfahren des größten Rests; bei Gleichstand der höhere Satz). |
| Versand | 4,90 € brutto innerhalb Deutschlands, **kostenlos ab 50,00 €** Warenwert **nach** Rabatt. Ohne Positionen fällt kein Versand an. Die Versandkosten werden im Verhältnis der rabattierten Warenwerte auf die Steuersätze verteilt (bei Warenwert 0 nach Stückzahl). |
| Umsatzsteuer | Je Steuersatz: Brutto = Warenwert − Rabattanteil + Versandanteil; USt = Brutto × Satz ÷ (100 % + Satz), kaufmännisch gerundet; Netto = Brutto − USt. Ausgewiesen wird je Satz, höchster Satz zuerst. |
| Gesamtsumme | Warenwert − Rabatt + Versand. Sie ist gleich der Summe der Bruttobeträge aller Steuerzeilen. |
| Rechenweise | Nur `int64`-Cent; Produkte mit 128-Bit-Zwischenwerten (`math/bits`), Überlauf wird abgewiesen. Keine Gleitkommazahlen. |

**Einordnung nach Produktart** (Stand 2026, [§ 12 UStG](https://www.gesetze-im-internet.de/ustg_1980/__12.html), [Anlage 2](https://www.gesetze-im-internet.de/ustg_1980/anlage_2.html)):

| Produktart | Klasse | Hinweis |
| --- | --- | --- |
| Küchen- und Haushaltswaren (z. B. Pfannen, Geschirr), Textilien, Elektronik, Möbel, Kosmetik | `standard` | Regelsatz; gilt für das heutige Sortiment „Küche & Haushalt“ |
| Gedruckte Bücher, Zeitungen, Zeitschriften | `reduced` | Anlage 2; nicht für jugendgefährdende Schriften und Druckwerke, die überwiegend Werbung enthalten |
| Hörbücher auf Datenträgern | `reduced` | Anlage 2 |
| Lebensmittel (die meisten) | `reduced` | Getränke wie Säfte, Mineralwasser und alkoholische Getränke sind in der Regel `standard`; je Artikel prüfen |
| Lebende Pflanzen und Schnittblumen | `reduced` | Anlage 2 |
| Sets mit Waren unterschiedlicher Sätze (z. B. Kochbuch mit Pfanne) | — | Mit einer Klasse je Produkt nicht abbildbar. Bis zur steuerlichen Prüfung nur als getrennte Produkte anbieten |
| Gutscheine, Pfand, digitale Inhalte | — | Nicht Teil dieses Vertrags; eigener ADR vor Einführung |

Die Klasse legt die Person fest, die das Produkt anlegt. Im Zweifel bleibt der Regelsatz, denn eine zu niedrige Steuer würde zur Nachzahlung führen.

**Rabattcodes:**

- Codes bestehen aus 3–32 Zeichen `A–Z`, `0–9` und `-`. Sie werden großgeschrieben gespeichert und sind nach dem Anlegen unveränderlich.
- Admins legen Codes an, ändern Satz, Aktivierung und Gültigkeit und deaktivieren sie, statt sie zu löschen. Jede Änderung schreibt ein Audit-Ereignis.
- Ein deaktivierter oder abgelaufener Code bleibt im Warenkorb sichtbar (`applied: false`), wird aber nicht angerechnet.
- Unbekannte, deaktivierte und abgelaufene Codes beantwortet die API einheitlich mit 404. Nach 10 Fehlversuchen je Kunde in 15 Minuten folgt 429. Das verhindert, dass Codes erraten werden. Gezählt wird in der Tabelle `auth_rate_limits` mit eigenem Schlüssel.

## Betrachtete Alternativen

- **Nettopreise speichern:** Die Umsatzsteuer käme bei jeder Position oben drauf. Bei Verbraucherpreisen entstehen so krumme Bruttobeträge und Cent-Abweichungen zur Anzeige. Abgelehnt.
- **Steuer je Position runden:** Bei vielen Positionen summieren sich Rundungsdifferenzen; die Rechnung weist ohnehin je Satz aus. Abgelehnt zugunsten der Rundung je Steuersatz.
- **Versand immer mit 19 %:** Einfacher, entspricht aber nicht der Behandlung als Nebenleistung bei gemischten Warenkörben. Abgelehnt.
- **Versand nach Nettowerten statt Bruttowerten aufteilen:** Ebenfalls eine sachgerechte Aufteilung; die Ergebnisse weichen nur um wenige Cent ab. Gewählt sind die Bruttowerte, weil Freigrenze und Anzeige ebenfalls brutto sind. Die steuerliche Prüfung vor NW-21 bestätigt die Wahl oder stellt auf Nettowerte um; das ist eine Änderung in `pricing.Calculate` und ihren Tests.
- **Steuersätze in der Datenbank:** Mehr Flexibilität, aber auch Risiko falscher Werte. Eine Satzänderung ist selten und gehört in Code, Test und Review. Abgelehnt.

## Folgen

- Positiv: Eine reine Go-Funktion (`pricing.Calculate`) berechnet alles. Warenkorb und später der Checkout nutzen sie. Die Oberfläche zeigt nur die Werte der API.
- Negativ: Migration 8 ändert `products`; ihr Down-Lauf verweigert Datenverlust bei vorhandenen Codes oder ermäßigten Produkten. Eine Satzänderung erfordert ein Release.
- Bestellungen (NW-16) müssen Steuerklasse, Satz, Rabattcode und die vollständige Preisaufstellung zum Bestellzeitpunkt mitspeichern, damit spätere Satz-, Preis- oder Codeänderungen alte Bestellungen nicht verändern. Der Checkout rechnet dafür mit `pricing.Calculate` neu und übernimmt keine Beträge aus dem Browser.

## Validierung

- `pricing_test.go`:
  - Beispiele für Freigrenze ±1 Cent, gemischte Sätze, kaufmännische Rundung, krumme Prozentsätze, 100 % Rabatt, kostenlose Ware sowie ungültige und überlaufende Eingaben.
  - Ein Kombinationstest über Preise, Mengen, Rabatte und Versandregeln prüft die Invarianten: Summen gehen auf, Netto + USt = Brutto, keine negativen Teile, deterministisch.
- `cart_pricing_integration_test.go` (PostgreSQL): Warenkorb-Aufstellung, Steuerklasse per Admin-API, Code-Pflege mit Rolle, CSRF und Audit, Ablauf und Deaktivierung, Fehlversuchsgrenze, Trennung der Kunden.
- Ende-zu-Ende gegen die echte API: Die Oberfläche zeigt exakt die Beträge der API, siehe [NW-15](../progress/NW-15.md).

## Wiedervorlage

- Bei einer Änderung der Steuersätze, bei Versand ins Ausland, bei Gutscheinen mit festem Betrag oder Mehrzweck-Gutscheinen, bei B2B-Kunden (Nettopreise) und vor dem ersten echten Verkauf (steuerliche Prüfung, NW-21).

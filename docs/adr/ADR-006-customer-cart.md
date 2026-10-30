# ADR-006: Kontogebundener Warenkorb

**Status:** Angenommen am 30. Oktober 2026 (NW-14).

## Entscheidung

Ein Warenkorb gehört genau einem angemeldeten Kundenkonto (`customer_id`). Er übersteht Logout und neue Sessions desselben Kontos. Anonyme Warenkörbe, Browser-Speicher und eine spätere Zusammenführung werden erst mit einer eigenen Anforderung eingeführt. Adminrollen haben keinen Kundenwarenkorb.

`cart_items` speichert nur Varianten-ID und Menge, keine Preise oder personenbezogenen Daten. Die Primärschlüssel-Kombination `(customer_id, variant_id)` verhindert doppelte Zeilen. `POST` erhöht die Menge derselben Variante atomar; `PUT` setzt sie vollständig; `DELETE` entfernt sie. Menge 1–99 und höchstens 50 unterschiedliche Varianten gelten pro Konto. Hinzufügen sperrt den Kundendatensatz für die Zeilengrenze und den Variantendatensatz für den Bestandsabgleich. Schreibzugriffe verlangen Session und CSRF-Token.

Jedes `GET` liefert aktuellen Namen, Preis und Bestand aus dem Katalog. Ein nicht mehr veröffentlichtes oder inaktives Produkt sowie eine Menge über dem aktuellen Bestand bleibt als nicht lieferbare Position sichtbar und kann angepasst oder entfernt werden. Wird eine Variante gelöscht, entfernt der FK mit `ON DELETE CASCADE` die zugehörige Zeile; ein fehlendes Produkt lässt die Seite nicht abstürzen. Die Zwischensumme zählt nur derzeit lieferbare Zeilen. Sie ist kein Checkout-Preis und reserviert keinen Bestand. Steuer, Versand, finaler Preis und paralleler Bestandsabbau werden in NW-15–17 entschieden.

Migration 7 ist additiv. Ihr Down-Lauf verweigert das Löschen nicht leerer Warenkörbe. Der Warenkorb wird mit `Cache-Control: no-store` beantwortet und zwischen Kunden allein über die serverseitig geprüfte Session getrennt.

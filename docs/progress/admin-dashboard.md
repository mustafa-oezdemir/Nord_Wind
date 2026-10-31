# NordWinds Admin Dashboard

Die Admin-Startseite zeigt sechs direkte Wege: Admin Dashboard, Kategoriebaum,
Produkte, Kategorie anlegen, Produkt anlegen und Rabattcodes. Bestellungen und
Versand bleiben über einen eigenen Link erreichbar. Mitarbeiter mit der Rolle
`employee` sehen weiterhin ausschließlich diesen Bestellbereich.

## Umsatzanalyse

Die Auswahl erfolgt in drei Schritten: Gesamt-, Kategorie- oder Produktumsatz;
7 Tage, 30 Tage oder drei Kalendermonate; anschließend Balken-, Linien-,
Flächen- oder Ringdiagramm. Eine Tabelle liefert dieselben Summen ohne
grafische Darstellung und bleibt per Tastatur und Screenreader zugänglich.
Die Diagramme werden mit `chart.js` im Browser erzeugt. Die Canvas-Grafik ist
für Screenreader ausgeblendet, weil die nachfolgende Tabelle alle Tages- bzw.
Monatswerte enthält. Für SSR wird Chart.js erst im Browser geladen.

Der Umsatz umfasst Bruttobeträge der Bestellpositionen nach anteiligem
Bestellrabatt, ohne Versandkosten. Maßgeblich ist das erste `PAID`-Ereignis
einer weiterhin bezahlten, bearbeiteten, versandten oder zugestellten
Bestellung in der Zeitzone `Europe/Berlin`. Erstattete Mock-Zahlungen werden
ausgeschlossen. Für Kategorien gilt die **aktuelle** erste Zuordnung nach
`sort_order` und ID; spätere Katalogänderungen können deshalb historische
Zuordnungen und Namen in der Analyse ändern. Maximal acht Reihen werden
einzeln gezeigt, weitere unter „Weitere“ zusammengefasst.

Die API `GET /api/v1/admin/analytics/sales` akzeptiert nur die dokumentierten
Werte für `group_by` und `period`, erfordert die Rolle `admin` und antwortet
mit `Cache-Control: no-store`.

## Prüfung

- PostgreSQL-Integration: Admin-Zugriff, Ablehnung für nicht angemeldete
  Personen, Kunden und Mitarbeiter; Zeitraum-Buckets; centgenaue
  Rabattverteilung; Kategorie- und Produktumsatz; Erstattungen.
- Angular-Komponententest: sechs Navigationsziele, Berechtigungsgrenze und
  Auswahlfolge mit Umsatzwert, Tabelle und Diagramm.

# ADR-010: Bestandsreservierung mit fester Sperrreihenfolge

- **Status:** Angenommen und an NW-16 angebunden am 30. Oktober 2026.
- **Betrifft:** `backend/internal/inventory`, `order_reservations`, Bestell- und Zahlungsübergänge.

## Kontext

Der Warenkorb reserviert nichts. Beim Checkout können mehrere Kunden gleichzeitig das letzte Stück einer Variante kaufen. Ein Preis- oder Bestandswert aus dem Browser ist dafür nicht maßgeblich. Eine offene Zahlung darf Bestand nur befristet blockieren.

## Entscheidung

Der Checkout erstellt Bestellkopf und unveränderliche Positions-Snapshots und ruft `inventory.Reserve` **in derselben PostgreSQL-Transaktion** auf. Vor dem ersten Varianten-Update sperrt er die Kundenzeile für die Idempotenzprüfung. `Reserve` sortiert die Varianten-IDs aufsteigend und führt für jede Position ein bedingtes `UPDATE product_variants SET stock_quantity = stock_quantity - quantity WHERE stock_quantity >= quantity` aus. Nur ein erfolgreich aktualisierter Datensatz gilt als reserviert. Danach schreibt es eine `order_reservations`-Zeile. Jeder Fehler rollt die gesamte Checkout-Transaktion zurück; ein teilweise reservierter Warenkorb darf nie committen.

Die Standardfrist beträgt 15 Minuten ab Checkout. Der Betrag und die Frist stehen in der Bestellung beziehungsweise Reservierung und werden bei wiederholter Anfrage mit demselben Idempotenzschlüssel nicht verlängert. `ReleaseExpired` sperrt je Bestellung den Bestellkopf mit `FOR UPDATE SKIP LOCKED` und gibt aktive Positionen in aufsteigender Varianten-ID genau einmal frei. Ohne gestartete Zahlung wird der Status `CANCELLED`, bei ausstehender Zahlung `PAYMENT_FAILED`. Ein periodischer Worker ruft diese Funktion auch ohne weitere Kundenanfrage auf. Ein Kundenabbruch über `POST /api/v1/orders/{orderNumber}/cancel` setzt `CANCELLED` und gibt aktive Positionen ebenfalls genau einmal frei. Jeder Übergang wird in `order_status_events` festgehalten.

Erfolgreiche Zahlung setzt die Reservierung auf `consumed` und belässt den zuvor abgebuchten Bestand. Ein Erfolg nach Fristablauf oder nach Freigabe darf die Bestellung nicht als bezahlt markieren. Zahlungsausfall gibt die aktive Reservierung frei. Eine Erstattung erhöht den Bestand nicht automatisch, weil der Zustand der Ware separat geprüft werden muss.

Alle Transaktionen, die mehr als eine Variante berühren, verwenden dieselbe ID-Reihenfolge. Ein PostgreSQL-Deadlock (`40P01`) oder Serialisierungsfehler (`40001`) wiederholt höchstens die vollständig zurückgerollte Datenbank-Transaktion. Externe Aufrufe, E-Mails und Zahlungen gehören nicht in diesen Retry-Block.

## Prüfung

Ein separater PostgreSQL-Test startet zwei gleichzeitige Reservierungen für das letzte Stück und verlangt genau einen Erfolg sowie nie negativen Bestand. Er prüft Ablauf, doppelte Freigabe, Abbruch, bezahlte Bestellungen und einen absichtlich erzeugten echten Deadlock mit anschließendem Retry. Derselbe Fall läuft zusätzlich über `POST /api/v1/orders` mit zwei verschiedenen Kunden: nur eine Bestellung gewinnt, nach Stornierung kann der andere Kunde denselben Versuch erfolgreich wiederholen.

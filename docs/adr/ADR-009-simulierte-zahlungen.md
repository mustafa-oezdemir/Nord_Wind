# ADR-009: Simulierte Zahlungsarten für den Entwicklungsbetrieb

- **Status:** Angenommen am 30. Oktober 2026 für NW-18.
- **Geltung:** Ausschließlich lokaler Entwicklungs- und Testbetrieb. Diese Entscheidung gibt keine echten Zahlungen frei.
- **Betrifft:** `backend/internal/payment`, die Bestell- und Reservierungsübergänge nach NW-16/NW-17 sowie den Mock-Webhook.

## Kontext

Der Projektinhaber möchte Debitkarte, Klarna, PayPal und Überweisung zunächst vollständig simulieren. NW-16 erstellt die Bestellung, NW-17 reserviert den Bestand. Die Zahlungsabwicklung soll diese Zustände unter realistischen Erfolg-, Fehler-, Abbruch- und Erstattungsereignissen testen, ohne Kartendaten oder Geld zu verarbeiten.

## Entscheidung

NordWinds verwendet für NW-18 einen eigenen, nur im Entwicklungsbetrieb aktivierten Mock-Anbieter mit den technischen Zahlungsarten `debit_card`, `klarna`, `paypal` und `bank_transfer`. Alle vier verwenden denselben Zustandsvertrag. Die Namen beschreiben Testoptionen; es gibt keine Verbindung zu den genannten Zahlungsdiensten.

Eine Zahlungsinitiierung referenziert eine bereits erstellte Bestellung und deren serverseitig gespeicherten EUR-Cent-Betrag. Sie erzeugt eine zufällige Zahlungs-ID und einen Status `PENDING`. Die Simulation kann daraufhin `succeeded`, `failed` oder nach einem Erfolg `refunded` auslösen. Nur ein signiertes Ereignis darf den Bestell- oder Zahlungsstatus ändern. Weder Kartennummern noch IBAN, Sicherheitscodes oder Zugangsdaten werden angenommen oder gespeichert.

Eine wiederholte Initiierung für dieselbe Bestellung und dieselbe Zahlungsart liefert dieselbe Zahlungs-ID. Ein Methodenwechsel für eine bereits initiierte Bestellung wird abgewiesen. Nach Fehler oder Reservierungsablauf bleibt die alte Bestellung abgeschlossen; ein neuer Zahlungsversuch benötigt einen neuen Checkout mit neuem Idempotenzschlüssel und einer neuen Bestandsprüfung.

Der Mock-Anbieter signiert die unveränderten JSON-Bytes mit HMAC-SHA256 über `timestamp + "." + body`. Der Header `X-Nordwinds-Mock-Signature` enthält `t=<Unix-Sekunden>,v1=<Hex-HMAC>`. Der Empfänger akzeptiert höchstens fünf Minuten Zeitabweichung und vergleicht die Signatur mit `hmac.Equal`. Ein ungültiger Header ergibt 401, ein gültig signiertes, aber ungültiges Ereignis 400. Eine eindeutige Provider-Ereignis-ID wird vor jeder Statusänderung transaktional gespeichert; erneute Zustellung hat keine weitere Wirkung.

Erfolg wandelt die aktive Reservierung in einen verkauften Bestand um. Fehlschlag oder Ablauf gibt sie genau einmal frei. Erstattung nach erfolgreicher Zahlung ändert den Zahlungsstatus, erhöht den Bestand aber nicht automatisch: Rückgabe, beschädigte Ware und bereits erfolgter Versand benötigen eine getrennte Lagerentscheidung. Späte Erfolgsmeldungen nach Reservierungsablauf dürfen eine freigegebene Bestellung nicht nachträglich als bezahlt markieren; sie werden als Abgleichfall erfasst.

## Grenzen und Verifikation

Der Mock-Anbieter ist kein Zahlungsdienst. Ein echter Anbieter, dessen Testkonto, signierte Webhooks und Zahlungsarten müssen vor einem Verkaufsstart gesondert integriert und Ende zu Ende geprüft werden. Bis dahin bleiben Checkout und Zahlung für echte Kunden deaktiviert.

NW-18 prüft alle vier Methoden, manipulierte/abgelaufene Signaturen, doppelte Ereignisse, Fehler- und Erstattungsübergänge sowie den Bestandsabgleich in einer separaten PostgreSQL-Testdatenbank. NW-17 prüft konkurrierende Checkouts und die Freigabe abgelaufener Reservierungen.

# ADR-005: Produktbilder in PostgreSQL

**Status:** Angenommen am 30. Oktober 2026 (NW-13).

## Entscheidung

Produktbilder liegen in `product_images.image_bytes` als `BYTEA` im selben PostgreSQL-Backup wie der Katalog. Es gibt keinen beschreibbaren Webroot und keinen vom Nutzer bestimmten Speicherpfad. Die API erzeugt UUIDs und öffentliche Ressourcenpfade aus diesen IDs. Eine spätere Auslagerung in Object Storage erfordert einen eigenen Migrations- und Betriebsentscheid.

Nur angemeldete Administratoren können Bilder über eine CSRF-geschützte Multipart-Route hochladen oder löschen. Öffentlich lesbar sind Bilder ausschließlich für veröffentlichte oder archivierte Produkte. Entwurfsbilder ergeben 404. Der Browser erhält immer `image/jpeg` mit `nosniff`; Originalname, MIME-Header und Rohdatei werden nicht ausgeliefert.

Die API akzeptiert JPEG und PNG nur, wenn deklarierter MIME-Typ, Dateisignatur und Decoder übereinstimmen. Pro Bild gelten 3 MiB, höchstens 2000 × 2000 Pixel und vier Millionen Pixel. Ein Produkt hat höchstens sechs Bilder; der Eltern-Datensatz wird vor Zählung und Einfügen gesperrt. Bilder werden vollständig dekodiert, auf weißem Hintergrund zusammengesetzt und als neues JPEG kodiert. Dadurch werden Metadaten und angehängte Fremddaten nicht übernommen. Transparenz wird weiß; EXIF-Drehung wird nicht ausgewertet.

Der Bilddatensatz und sein Audit-Ereignis entstehen in einer Transaktion. Die öffentliche Liste liefert das erste Bild, die Detailseite alle Bilder in Uploadreihenfolge. Fehlt ein Bild oder schlägt das Laden fehl, zeigen Karten und Galerie das lokale Platzhalter-Motiv. Nach Löschen kann ein Browser ein Bild höchstens fünf Minuten aus seinem Cache anzeigen.

## Folgen

PostgreSQL-Speicher und Backups wachsen mit Bildern. Die festen Grenzen halten die Größe pro Produkt bei höchstens 18 MiB. Migration 6 ist additiv; ihr Down-Lauf verweigert Datenverlust bei vorhandenen Bildern oder Bild-Audit-Ereignissen. SVG, GIF, WebP und externe Bild-URLs sind nicht zugelassen. Für produktiven Betrieb sind Backup-Größe, Bildrecht, Moderation und gegebenenfalls Malware-Scanning organisatorisch zu klären.

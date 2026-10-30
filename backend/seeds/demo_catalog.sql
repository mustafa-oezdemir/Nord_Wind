-- Freigegebene Beispieldaten für den lokalen NordWinds-Katalog.
-- Wiederholbar: vorhandene Datensätze werden nicht überschrieben.
BEGIN;

WITH seed (nr, name, slug, description) AS (
    VALUES
        (1, 'Küche & Haushalt', 'kueche-haushalt', 'Praktische Artikel für Küche und Haushalt.'),
        (2, 'Wohnen', 'wohnen', 'Nützliche und gemütliche Wohnaccessoires.'),
        (3, 'Garten', 'garten', 'Ausstattung für Pflanzen und Gartenpflege.'),
        (4, 'Outdoor', 'outdoor', 'Begleiter für Ausflüge und unterwegs.'),
        (5, 'Sport & Fitness', 'sport-fitness', 'Zubehör für Bewegung und Training.'),
        (6, 'Elektronik', 'elektronik', 'Technik für den Alltag.'),
        (7, 'Büro', 'buero', 'Hilfsmittel für einen aufgeräumten Arbeitsplatz.'),
        (8, 'Bad & Pflege', 'bad-pflege', 'Alltagsartikel für Bad und Pflege.'),
        (9, 'Kinder & Spiel', 'kinder-spiel', 'Spiel- und Kreativartikel für Kinder.'),
        (10, 'Haustierbedarf', 'haustierbedarf', 'Zubehör für Haustiere.')
)
INSERT INTO categories (id, name, slug, description, status, sort_order)
SELECT ('a4100000-0000-4000-8000-' || lpad(nr::text, 12, '0'))::uuid,
       name, slug, description, 'active', nr * 10
FROM seed
ON CONFLICT DO NOTHING;

WITH seed (nr, name, slug, description) AS (
    VALUES
        (1, 'Gusseisenpfanne 28 cm', 'gusseisenpfanne-28-cm', 'Pfanne aus Gusseisen mit 28 cm Durchmesser.'),
        (2, 'Vorratsglas 1 l', 'vorratsglas-1-l', 'Vorratsglas mit Deckel für trockene Lebensmittel.'),
        (3, 'Kissenhülle 45 × 45 cm', 'kissenhuelle-45x45-cm', 'Kissenhülle aus Baumwolle für Wohn- und Schlafzimmer.'),
        (4, 'LED-Tischleuchte', 'led-tischleuchte', 'Kompakte Tischleuchte mit LED-Licht.'),
        (5, 'Pflanzkasten 80 cm', 'pflanzkasten-80-cm', 'Pflanzkasten für Balkon oder Terrasse.'),
        (6, 'Gartenschere 20 cm', 'gartenschere-20-cm', 'Handliche Schere für leichte Gartenarbeiten.'),
        (7, 'Edelstahl-Trinkflasche 750 ml', 'edelstahl-trinkflasche-750-ml', 'Wiederverwendbare Trinkflasche für unterwegs.'),
        (8, 'Picknickdecke 150 × 200 cm', 'picknickdecke-150x200-cm', 'Decke für Picknick und Ausflug.'),
        (9, 'Yogamatte 6 mm', 'yogamatte-6-mm', 'Matte für Yoga und Bodenübungen.'),
        (10, 'Widerstandsband-Set', 'widerstandsband-set', 'Set mit Bändern für verschiedene Trainingsübungen.'),
        (11, 'USB-C-Ladegerät 30 W', 'usb-c-ladegeraet-30-w', 'Kompaktes Ladegerät mit USB-C-Anschluss.'),
        (12, 'Bluetooth-Lautsprecher', 'bluetooth-lautsprecher', 'Tragbarer Lautsprecher mit Bluetooth-Verbindung.'),
        (13, 'Notizbuch A5', 'notizbuch-a5', 'Notizbuch im Format A5 für Alltag und Büro.'),
        (14, 'Schreibtisch-Organizer', 'schreibtisch-organizer', 'Organizer für Stifte und kleine Büroartikel.'),
        (15, 'Baumwoll-Handtuch 70 × 140 cm', 'baumwoll-handtuch-70x140-cm', 'Handtuch aus Baumwolle für das Badezimmer.'),
        (16, 'Seifenspender 300 ml', 'seifenspender-300-ml', 'Nachfüllbarer Spender für Flüssigseife.'),
        (17, 'Holzbausteine 50 Stück', 'holzbausteine-50-stueck', 'Baustein-Set aus Holz zum freien Spielen.'),
        (18, 'Malset 24 Farben', 'malset-24-farben', 'Malset mit 24 Farben für kreative Projekte.'),
        (19, 'Futternapf Edelstahl 700 ml', 'futternapf-edelstahl-700-ml', 'Futternapf aus Edelstahl für Haustiere.'),
        (20, 'Hundeleine 2 m', 'hundeleine-2-m', 'Leine für Spaziergänge mit Hunden.')
)
INSERT INTO products (id, name, slug, description, status)
SELECT ('a4200000-0000-4000-8000-' || lpad(nr::text, 12, '0'))::uuid,
       name, slug, description, 'published'
FROM seed
ON CONFLICT DO NOTHING;

INSERT INTO product_categories (product_id, category_id)
SELECT ('a4200000-0000-4000-8000-' || lpad(nr::text, 12, '0'))::uuid,
       ('a4100000-0000-4000-8000-' || lpad(((nr - 1) / 2 + 1)::text, 12, '0'))::uuid
FROM generate_series(1, 20) AS nr
ON CONFLICT DO NOTHING;

WITH seed (nr, sku, price_cents, stock_quantity) AS (
    VALUES
        (1, 'NW-DEMO-PFANNE-28', 5990, 18),
        (2, 'NW-DEMO-GLAS-1L', 1490, 36),
        (3, 'NW-DEMO-KISSEN-45', 1890, 28),
        (4, 'NW-DEMO-LEUCHTE', 3990, 16),
        (5, 'NW-DEMO-PFLANZKASTEN-80', 4990, 14),
        (6, 'NW-DEMO-GARTENSCHERE', 2490, 25),
        (7, 'NW-DEMO-FLASCHE-750', 2290, 40),
        (8, 'NW-DEMO-PICKNICKDECKE', 3490, 20),
        (9, 'NW-DEMO-YOGAMATTE-6', 2990, 30),
        (10, 'NW-DEMO-BANDSET', 1990, 32),
        (11, 'NW-DEMO-LADEGERAET-30', 2490, 27),
        (12, 'NW-DEMO-LAUTSPRECHER', 4990, 19),
        (13, 'NW-DEMO-NOTIZBUCH-A5', 1190, 50),
        (14, 'NW-DEMO-ORGANIZER', 1890, 33),
        (15, 'NW-DEMO-HANDTUCH-70', 1990, 42),
        (16, 'NW-DEMO-SEIFENSPENDER', 1490, 38),
        (17, 'NW-DEMO-BAUSTEINE-50', 3990, 17),
        (18, 'NW-DEMO-MALSET-24', 1290, 45),
        (19, 'NW-DEMO-FUTTERNAPF-700', 1690, 35),
        (20, 'NW-DEMO-HUNDELEINE-2M', 2190, 29)
)
INSERT INTO product_variants (id, product_id, sku, label, price_cents, currency, stock_quantity, is_active)
SELECT ('a4300000-0000-4000-8000-' || lpad(nr::text, 12, '0'))::uuid,
       ('a4200000-0000-4000-8000-' || lpad(nr::text, 12, '0'))::uuid,
       sku, 'Standard', price_cents, 'EUR', stock_quantity, true
FROM seed
ON CONFLICT DO NOTHING;

DO $$
BEGIN
    IF (SELECT count(*) FROM categories WHERE id::text LIKE 'a4100000-0000-4000-8000-%') <> 10
       OR (SELECT count(*) FROM products WHERE id::text LIKE 'a4200000-0000-4000-8000-%') <> 20
       OR (SELECT count(*) FROM product_variants WHERE id::text LIKE 'a4300000-0000-4000-8000-%') <> 20
       OR (SELECT count(*) FROM product_categories WHERE product_id::text LIKE 'a4200000-0000-4000-8000-%') <> 20 THEN
        RAISE EXCEPTION 'NordWinds-Beispielkatalog unvollständig; Transaktion wird zurückgenommen';
    END IF;
END $$;

COMMIT;

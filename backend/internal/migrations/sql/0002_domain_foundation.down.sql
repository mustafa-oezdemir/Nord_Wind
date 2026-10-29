LOCK TABLE order_items, orders, customers, product_variants, product_categories, products, categories
    IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM order_items LIMIT 1)
        OR EXISTS (SELECT 1 FROM orders LIMIT 1)
        OR EXISTS (SELECT 1 FROM customers LIMIT 1)
        OR EXISTS (SELECT 1 FROM product_variants LIMIT 1)
        OR EXISTS (SELECT 1 FROM product_categories LIMIT 1)
        OR EXISTS (SELECT 1 FROM products LIMIT 1)
        OR EXISTS (SELECT 1 FROM categories LIMIT 1) THEN
        RAISE EXCEPTION 'Domain-Tabellen enthalten Daten; Rollback verweigert';
    END IF;
END $$;

DROP TABLE order_items;
DROP TABLE orders;
DROP TABLE customers;
DROP TABLE product_variants;
DROP TABLE product_categories;
DROP TABLE products;
DROP TABLE categories;
DROP FUNCTION nordwinds_prevent_category_cycle();

LOCK TABLE order_status_events, order_items, orders IN ACCESS EXCLUSIVE MODE;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM orders) OR EXISTS (SELECT 1 FROM order_items)
        OR EXISTS (SELECT 1 FROM order_status_events) THEN
        RAISE EXCEPTION 'Bestellungen vorhanden; Down verweigert Datenverlust';
    END IF;
END $$;
DROP TRIGGER orders_guard ON orders;
DROP FUNCTION nordwinds_guard_order();
DROP TRIGGER order_items_append_only ON order_items;
DROP TABLE order_status_events;
DROP FUNCTION nordwinds_append_only();
ALTER TABLE order_items
    DROP CONSTRAINT order_items_position_unique,
    DROP CONSTRAINT order_items_tax_fields,
    DROP CONSTRAINT order_items_line_total_matches,
    DROP COLUMN line_total_cents,
    DROP COLUMN variant_label_snapshot,
    DROP COLUMN position;
DROP INDEX orders_customer_idempotency_unique;
ALTER TABLE orders
    DROP CONSTRAINT orders_shipping_address,
    DROP CONSTRAINT orders_tax_lines_valid,
    DROP CONSTRAINT orders_discount_percent_range,
    DROP CONSTRAINT orders_discount_pair,
    DROP CONSTRAINT orders_total_matches,
    DROP CONSTRAINT orders_amounts_valid,
    DROP COLUMN tax_lines,
    DROP COLUMN discount_percent_bp,
    DROP COLUMN discount_code,
    DROP COLUMN shipping_cents,
    DROP COLUMN discount_cents,
    DROP COLUMN goods_cents,
    DROP COLUMN customer_email,
    DROP COLUMN request_hash,
    DROP COLUMN idempotency_key;
DROP SEQUENCE order_number_seq;

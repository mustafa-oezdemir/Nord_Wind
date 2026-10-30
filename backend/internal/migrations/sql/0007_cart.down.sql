DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cart_items) THEN
        RAISE EXCEPTION 'cart_items enthält Daten; Down verweigert Datenverlust';
    END IF;
END $$;
DROP TABLE cart_items;

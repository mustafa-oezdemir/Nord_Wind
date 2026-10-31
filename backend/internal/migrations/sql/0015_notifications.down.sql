LOCK TABLE notification_outbox IN ACCESS EXCLUSIVE MODE;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM notification_outbox) THEN
        RAISE EXCEPTION 'notification_outbox enthält Benachrichtigungen; Down verweigert Datenverlust';
    END IF;
END $$;
DROP TRIGGER customers_notify ON customers;
DROP FUNCTION nordwinds_notify_customer();
DROP TRIGGER mock_payments_notify_refund ON mock_payments;
DROP FUNCTION nordwinds_notify_refund();
DROP TRIGGER order_status_events_notify ON order_status_events;
DROP FUNCTION nordwinds_notify_order_status();
DROP TABLE notification_outbox;

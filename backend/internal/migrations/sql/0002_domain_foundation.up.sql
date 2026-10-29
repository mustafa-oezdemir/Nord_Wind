CREATE TABLE categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id UUID REFERENCES categories(id) ON DELETE RESTRICT,
    name TEXT NOT NULL CONSTRAINT categories_name_nonempty CHECK (btrim(name) <> ''),
    slug TEXT NOT NULL CONSTRAINT categories_slug_nonempty CHECK (btrim(slug) <> '' AND slug = btrim(slug)),
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CONSTRAINT categories_status_valid CHECK (status IN ('active', 'inactive')),
    sort_order INTEGER NOT NULL DEFAULT 0 CONSTRAINT categories_sort_order_nonnegative CHECK (sort_order >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT categories_not_self_parent CHECK (parent_id IS NULL OR parent_id <> id)
);

CREATE UNIQUE INDEX categories_slug_ci_unique ON categories (lower(slug));
CREATE INDEX categories_parent_idx ON categories (parent_id);
CREATE INDEX categories_status_sort_idx ON categories (status, sort_order, id);

CREATE FUNCTION nordwinds_prevent_category_cycle() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.parent_id IS NULL THEN
        RETURN NEW;
    END IF;
    PERFORM pg_advisory_xact_lock(703002);
    IF EXISTS (
        WITH RECURSIVE ancestors(id, parent_id) AS (
            SELECT id, parent_id FROM categories WHERE id = NEW.parent_id
            UNION
            SELECT c.id, c.parent_id FROM categories c JOIN ancestors a ON c.id = a.parent_id
        )
        SELECT 1 FROM ancestors WHERE id = NEW.id
    ) THEN
        RAISE EXCEPTION 'Kategoriezyklus ist nicht erlaubt'
            USING ERRCODE = '23514', CONSTRAINT = 'categories_no_cycle';
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER categories_prevent_cycle
    BEFORE INSERT OR UPDATE OF parent_id ON categories
    FOR EACH ROW EXECUTE FUNCTION nordwinds_prevent_category_cycle();

CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL CONSTRAINT products_name_nonempty CHECK (btrim(name) <> ''),
    slug TEXT NOT NULL CONSTRAINT products_slug_nonempty CHECK (btrim(slug) <> '' AND slug = btrim(slug)),
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft' CONSTRAINT products_status_valid CHECK (status IN ('draft', 'published', 'archived')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX products_slug_ci_unique ON products (lower(slug));
CREATE INDEX products_status_created_idx ON products (status, created_at DESC, id);

CREATE TABLE product_categories (
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    PRIMARY KEY (product_id, category_id)
);

CREATE INDEX product_categories_category_idx ON product_categories (category_id, product_id);

CREATE TABLE product_variants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    sku TEXT NOT NULL CONSTRAINT product_variants_sku_nonempty CHECK (btrim(sku) <> '' AND sku = btrim(sku)),
    label TEXT NOT NULL CONSTRAINT product_variants_label_nonempty CHECK (btrim(label) <> ''),
    price_cents BIGINT NOT NULL CONSTRAINT product_variants_price_nonnegative CHECK (price_cents >= 0),
    currency CHAR(3) NOT NULL DEFAULT 'EUR' CONSTRAINT product_variants_currency_eur CHECK (currency = 'EUR'),
    stock_quantity INTEGER NOT NULL DEFAULT 0 CONSTRAINT product_variants_stock_nonnegative CHECK (stock_quantity >= 0),
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX product_variants_sku_ci_unique ON product_variants (lower(sku));
CREATE INDEX product_variants_product_idx ON product_variants (product_id, id);

-- Der Identitäts- und Session-Lebenszyklus folgt in NW-11; hier entsteht nur der FK-Anker.
CREATE TABLE customers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL CONSTRAINT customers_email_nonempty CHECK (btrim(email) <> '' AND email = btrim(email)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX customers_email_ci_unique ON customers (lower(email));

CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_number TEXT NOT NULL UNIQUE CONSTRAINT orders_number_nonempty CHECK (btrim(order_number) <> ''),
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'CREATED' CONSTRAINT orders_status_valid CHECK (
        status IN ('CREATED', 'PAYMENT_PENDING', 'PAID', 'PROCESSING', 'SHIPPED', 'DELIVERED', 'CANCELLED', 'PAYMENT_FAILED')
    ),
    total_cents BIGINT NOT NULL CONSTRAINT orders_total_nonnegative CHECK (total_cents >= 0),
    currency CHAR(3) NOT NULL DEFAULT 'EUR' CONSTRAINT orders_currency_eur CHECK (currency = 'EUR'),
    shipping_snapshot JSONB NOT NULL CONSTRAINT orders_shipping_object CHECK (jsonb_typeof(shipping_snapshot) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX orders_customer_created_idx ON orders (customer_id, created_at DESC, id);
CREATE INDEX orders_status_created_idx ON orders (status, created_at DESC, id);

CREATE TABLE order_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    variant_id UUID NOT NULL REFERENCES product_variants(id) ON DELETE RESTRICT,
    name_snapshot TEXT NOT NULL CONSTRAINT order_items_name_nonempty CHECK (btrim(name_snapshot) <> ''),
    sku_snapshot TEXT NOT NULL CONSTRAINT order_items_sku_nonempty CHECK (btrim(sku_snapshot) <> ''),
    quantity INTEGER NOT NULL CONSTRAINT order_items_quantity_positive CHECK (quantity > 0),
    unit_price_cents BIGINT NOT NULL CONSTRAINT order_items_price_nonnegative CHECK (unit_price_cents >= 0),
    currency CHAR(3) NOT NULL DEFAULT 'EUR' CONSTRAINT order_items_currency_eur CHECK (currency = 'EUR'),
    tax_snapshot JSONB NOT NULL CONSTRAINT order_items_tax_object CHECK (jsonb_typeof(tax_snapshot) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (order_id, variant_id)
);

CREATE INDEX order_items_variant_idx ON order_items (variant_id);

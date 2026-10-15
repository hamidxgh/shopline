CREATE TABLE IF NOT EXISTS order_events
(
    order_id   UInt64,
    product_id UInt64,
    cents      Int64,
    created_at DateTime DEFAULT now()
)
ENGINE = MergeTree
ORDER BY (order_id, product_id);

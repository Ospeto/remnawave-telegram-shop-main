-- Add revenue tracking columns to purchase table
ALTER TABLE purchase ADD COLUMN IF NOT EXISTS plan_label TEXT DEFAULT '';
ALTER TABLE purchase ADD COLUMN IF NOT EXISTS payment_method VARCHAR(20) DEFAULT '';
ALTER TABLE purchase ADD COLUMN IF NOT EXISTS payment_phone TEXT DEFAULT '';
ALTER TABLE purchase ADD COLUMN IF NOT EXISTS verified_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE purchase ADD COLUMN IF NOT EXISTS transaction_id TEXT DEFAULT '';

-- Create index for revenue queries
CREATE INDEX IF NOT EXISTS idx_purchase_paid_at ON purchase(paid_at);
CREATE INDEX IF NOT EXISTS idx_purchase_status ON purchase(status);

-- Revenue daily summary view
CREATE OR REPLACE VIEW revenue_daily AS
SELECT
    DATE(paid_at) AS day,
    payment_method,
    currency,
    COUNT(*) AS total_purchases,
    SUM(amount) AS total_revenue,
    COUNT(DISTINCT customer_id) AS unique_customers
FROM purchase
WHERE status = 'paid' AND paid_at IS NOT NULL
GROUP BY DATE(paid_at), payment_method, currency
ORDER BY day DESC;

-- Revenue monthly summary view
CREATE OR REPLACE VIEW revenue_monthly AS
SELECT
    DATE_TRUNC('month', paid_at)::DATE AS month,
    payment_method,
    currency,
    COUNT(*) AS total_purchases,
    SUM(amount) AS total_revenue,
    COUNT(DISTINCT customer_id) AS unique_customers
FROM purchase
WHERE status = 'paid' AND paid_at IS NOT NULL
GROUP BY DATE_TRUNC('month', paid_at), payment_method, currency
ORDER BY month DESC;

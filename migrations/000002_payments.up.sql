CREATE TABLE payments (
    id                      UUID PRIMARY KEY,
    order_id                VARCHAR(64) NOT NULL UNIQUE,
    budget_id               UUID NOT NULL REFERENCES budgets(id),
    amount                  NUMERIC(10,2) NOT NULL CHECK (amount >= 0),
    status                  VARCHAR(20) NOT NULL CHECK (status IN ('pendente', 'aprovado', 'recusado', 'expirado', 'estornado')),
    checkout_url            TEXT,
    provider_preference_id  VARCHAR(64),
    provider_payment_id     VARCHAR(64),
    failure_reason          TEXT,
    saga_id                 VARCHAR(64),
    correlation_id          VARCHAR(64),
    expires_at              TIMESTAMPTZ NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_payments_pending ON payments (status, expires_at);

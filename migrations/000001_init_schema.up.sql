CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE budgets (
    id              UUID PRIMARY KEY,
    order_id        VARCHAR(64) NOT NULL UNIQUE,
    status          VARCHAR(20) NOT NULL CHECK (status IN ('gerado', 'aprovado', 'cancelado')),
    total_services  NUMERIC(10,2) NOT NULL,
    total_parts     NUMERIC(10,2) NOT NULL,
    total           NUMERIC(10,2) NOT NULL,
    saga_id         VARCHAR(64),
    correlation_id  VARCHAR(64),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE budget_items (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    budget_id     UUID NOT NULL REFERENCES budgets(id) ON DELETE CASCADE,
    position      INT NOT NULL,
    kind          VARCHAR(10) NOT NULL CHECK (kind IN ('servico', 'peca')),
    reference_id  VARCHAR(64) NOT NULL,
    name          VARCHAR(255) NOT NULL,
    quantity      INT NOT NULL CHECK (quantity > 0),
    unit_price    NUMERIC(10,2) NOT NULL CHECK (unit_price >= 0)
);

CREATE INDEX idx_budget_items_budget_id ON budget_items (budget_id);

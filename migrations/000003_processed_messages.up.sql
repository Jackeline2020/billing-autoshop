CREATE TABLE processed_messages (
    message_id    VARCHAR(64) PRIMARY KEY,
    processed_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

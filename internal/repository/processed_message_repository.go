package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ProcessedMessageRepository guarda os IDs das mensagens já processadas,
// para ignorar entregas repetidas do SQS.
type ProcessedMessageRepository interface {
	Exists(messageID string) (bool, error)
	MarkProcessed(messageID string) error
}

type ProcessedMessagePostgresRepository struct {
	db *pgxpool.Pool
}

func NewProcessedMessagePostgresRepository(db *pgxpool.Pool) *ProcessedMessagePostgresRepository {
	return &ProcessedMessagePostgresRepository{db: db}
}

func (r *ProcessedMessagePostgresRepository) Exists(messageID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM processed_messages WHERE message_id = $1)`, messageID).Scan(&exists)
	return exists, err
}

func (r *ProcessedMessagePostgresRepository) MarkProcessed(messageID string) error {
	_, err := r.db.Exec(context.Background(),
		`INSERT INTO processed_messages (message_id) VALUES ($1) ON CONFLICT DO NOTHING`, messageID)
	return err
}

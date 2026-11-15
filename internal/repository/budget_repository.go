package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"billing-autoshop/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrConcurrentUpdate indica que o registro mudou de status entre a
// leitura e a gravação.
var ErrConcurrentUpdate = errors.New("o registro foi alterado por outra operação; tente novamente")

const uniqueViolation = "23505"

type BudgetRepository interface {
	Create(b domain.Budget) error
	FindByOrderID(orderID string) (domain.Budget, error)
	UpdateStatus(b domain.Budget, previous domain.BudgetStatus) error
}

type BudgetPostgresRepository struct {
	db *pgxpool.Pool
}

func NewBudgetPostgresRepository(db *pgxpool.Pool) *BudgetPostgresRepository {
	return &BudgetPostgresRepository{db: db}
}

// Create grava o orçamento e seus itens na mesma transação.
func (r *BudgetPostgresRepository) Create(b domain.Budget) error {
	ctx := context.Background()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO budgets (id, order_id, status, total_services, total_parts, total, saga_id, correlation_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, b.ID, b.OrderID, b.Status, b.TotalServices, b.TotalParts, b.Total, b.SagaID, b.CorrelationID, b.CreatedAt, b.UpdatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return domain.ErrBudgetExists
	}
	if err != nil {
		return fmt.Errorf("erro ao criar orçamento: %w", err)
	}

	for i, it := range b.Items {
		_, err = tx.Exec(ctx, `
			INSERT INTO budget_items (budget_id, position, kind, reference_id, name, quantity, unit_price)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, b.ID, i, it.Kind, it.ReferenceID, it.Name, it.Quantity, it.UnitPrice)
		if err != nil {
			return fmt.Errorf("erro ao gravar item do orçamento: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *BudgetPostgresRepository) FindByOrderID(orderID string) (domain.Budget, error) {
	ctx := context.Background()
	var b domain.Budget
	var sagaID, correlationID *string
	err := r.db.QueryRow(ctx, `
		SELECT id, order_id, status, total_services, total_parts, total, saga_id, correlation_id, created_at, updated_at
		FROM budgets WHERE order_id = $1
	`, orderID).Scan(&b.ID, &b.OrderID, &b.Status, &b.TotalServices, &b.TotalParts, &b.Total, &sagaID, &correlationID, &b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Budget{}, domain.ErrBudgetNotFound
	}
	if err != nil {
		return domain.Budget{}, err
	}
	if sagaID != nil {
		b.SagaID = *sagaID
	}
	if correlationID != nil {
		b.CorrelationID = *correlationID
	}

	rows, err := r.db.Query(ctx, `
		SELECT kind, reference_id, name, quantity, unit_price
		FROM budget_items WHERE budget_id = $1 ORDER BY position
	`, b.ID)
	if err != nil {
		return domain.Budget{}, err
	}
	defer rows.Close()

	b.Items = make([]domain.BudgetItem, 0)
	for rows.Next() {
		var it domain.BudgetItem
		if err := rows.Scan(&it.Kind, &it.ReferenceID, &it.Name, &it.Quantity, &it.UnitPrice); err != nil {
			return domain.Budget{}, err
		}
		b.Items = append(b.Items, it)
	}
	return b, rows.Err()
}

// UpdateStatus grava o novo status somente se o status no banco ainda for
// o anterior.
func (r *BudgetPostgresRepository) UpdateStatus(b domain.Budget, previous domain.BudgetStatus) error {
	tag, err := r.db.Exec(context.Background(), `
		UPDATE budgets SET status = $1, updated_at = $2 WHERE id = $3 AND status = $4
	`, b.Status, b.UpdatedAt.UTC().Truncate(time.Microsecond), b.ID, previous)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConcurrentUpdate
	}
	return nil
}

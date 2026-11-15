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

type PaymentRepository interface {
	Create(p domain.Payment) error
	FindByOrderID(orderID string) (domain.Payment, error)
	Update(p domain.Payment, previous domain.PaymentStatus) error
	FindExpired(now time.Time) ([]domain.Payment, error)
}

type PaymentPostgresRepository struct {
	db *pgxpool.Pool
}

func NewPaymentPostgresRepository(db *pgxpool.Pool) *PaymentPostgresRepository {
	return &PaymentPostgresRepository{db: db}
}

const paymentColumns = `id, order_id, budget_id, amount, status, checkout_url, provider_preference_id,
	provider_payment_id, failure_reason, saga_id, correlation_id, expires_at, created_at, updated_at`

func (r *PaymentPostgresRepository) Create(p domain.Payment) error {
	_, err := r.db.Exec(context.Background(), `
		INSERT INTO payments (`+paymentColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, p.ID, p.OrderID, p.BudgetID, p.Amount, p.Status, p.CheckoutURL, p.ProviderPreferenceID,
		p.ProviderPaymentID, p.FailureReason, p.SagaID, p.CorrelationID, p.ExpiresAt, p.CreatedAt, p.UpdatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return domain.ErrPaymentExists
	}
	if err != nil {
		return fmt.Errorf("erro ao criar pagamento: %w", err)
	}
	return nil
}

func (r *PaymentPostgresRepository) FindByOrderID(orderID string) (domain.Payment, error) {
	row := r.db.QueryRow(context.Background(), `SELECT `+paymentColumns+` FROM payments WHERE order_id = $1`, orderID)
	p, err := scanPayment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Payment{}, domain.ErrPaymentNotFound
	}
	return p, err
}

// Update grava a mudança somente se o status no banco ainda for o anterior
// (evita que o webhook e a expiração alterem o mesmo pagamento ao mesmo
// tempo).
func (r *PaymentPostgresRepository) Update(p domain.Payment, previous domain.PaymentStatus) error {
	tag, err := r.db.Exec(context.Background(), `
		UPDATE payments
		SET status = $1, provider_payment_id = $2, failure_reason = $3, updated_at = $4
		WHERE id = $5 AND status = $6
	`, p.Status, p.ProviderPaymentID, p.FailureReason, p.UpdatedAt, p.ID, previous)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConcurrentUpdate
	}
	return nil
}

// FindExpired devolve os pagamentos ainda pendentes cujo prazo já passou.
func (r *PaymentPostgresRepository) FindExpired(now time.Time) ([]domain.Payment, error) {
	rows, err := r.db.Query(context.Background(), `
		SELECT `+paymentColumns+` FROM payments
		WHERE status = $1 AND expires_at < $2
		ORDER BY expires_at
	`, domain.PaymentPending, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.Payment, 0)
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func scanPayment(row pgx.Row) (domain.Payment, error) {
	var p domain.Payment
	var checkoutURL, preferenceID, providerID, reason, sagaID, correlationID *string
	err := row.Scan(&p.ID, &p.OrderID, &p.BudgetID, &p.Amount, &p.Status, &checkoutURL, &preferenceID,
		&providerID, &reason, &sagaID, &correlationID, &p.ExpiresAt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return domain.Payment{}, err
	}
	p.CheckoutURL = deref(checkoutURL)
	p.ProviderPreferenceID = deref(preferenceID)
	p.ProviderPaymentID = deref(providerID)
	p.FailureReason = deref(reason)
	p.SagaID = deref(sagaID)
	p.CorrelationID = deref(correlationID)
	return p, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

package domain

import (
	"errors"
	"fmt"
	"time"
)

type PaymentStatus string

const (
	PaymentPending  PaymentStatus = "pendente"
	PaymentApproved PaymentStatus = "aprovado"
	PaymentRejected PaymentStatus = "recusado"
	PaymentExpired  PaymentStatus = "expirado"
	PaymentRefunded PaymentStatus = "estornado"
)

var (
	ErrPaymentNotFound = errors.New("pagamento não encontrado")
	ErrPaymentExists   = errors.New("já existe pagamento para esta OS")
	ErrBudgetCancelled = errors.New("o orçamento desta OS foi cancelado")
)

// PaymentTransitionError indica uma mudança de status fora do fluxo.
type PaymentTransitionError struct {
	From, To PaymentStatus
}

func (e *PaymentTransitionError) Error() string {
	return fmt.Sprintf("transição de pagamento inválida: %s → %s", e.From, e.To)
}

// Payment é a cobrança de uma OS. O cliente paga pelo link (CheckoutURL);
// o resultado chega pelo webhook do provedor.
type Payment struct {
	ID                   string        `json:"id"`
	OrderID              string        `json:"order_id"`
	BudgetID             string        `json:"budget_id"`
	Amount               float64       `json:"amount"`
	Status               PaymentStatus `json:"status"`
	CheckoutURL          string        `json:"checkout_url,omitempty"`
	ProviderPreferenceID string        `json:"provider_preference_id,omitempty"`
	ProviderPaymentID    string        `json:"provider_payment_id,omitempty"`
	FailureReason        string        `json:"failure_reason,omitempty"`
	SagaID               string        `json:"saga_id,omitempty"`
	CorrelationID        string        `json:"correlation_id,omitempty"`
	ExpiresAt            time.Time     `json:"expires_at"`
	CreatedAt            time.Time     `json:"created_at"`
	UpdatedAt            time.Time     `json:"updated_at"`
}

func NewPayment(id string, budget Budget, preferenceID, checkoutURL string, timeout time.Duration) Payment {
	now := time.Now().UTC()
	return Payment{
		ID:                   id,
		OrderID:              budget.OrderID,
		BudgetID:             budget.ID,
		Amount:               budget.Total,
		Status:               PaymentPending,
		CheckoutURL:          checkoutURL,
		ProviderPreferenceID: preferenceID,
		SagaID:               budget.SagaID,
		CorrelationID:        budget.CorrelationID,
		ExpiresAt:            now.Add(timeout),
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

// IsFinal indica que o pagamento não muda mais por ação do cliente.
func (p *Payment) IsFinal() bool {
	return p.Status != PaymentPending
}

func (p *Payment) Approve(providerPaymentID string) error {
	if err := p.transition(PaymentPending, PaymentApproved); err != nil {
		return err
	}
	p.ProviderPaymentID = providerPaymentID
	return nil
}

func (p *Payment) Reject(providerPaymentID, reason string) error {
	if err := p.transition(PaymentPending, PaymentRejected); err != nil {
		return err
	}
	p.ProviderPaymentID = providerPaymentID
	p.FailureReason = reason
	return nil
}

func (p *Payment) Expire() error {
	if err := p.transition(PaymentPending, PaymentExpired); err != nil {
		return err
	}
	p.FailureReason = "pagamento não confirmado dentro do prazo"
	return nil
}

func (p *Payment) Refund() error {
	return p.transition(PaymentApproved, PaymentRefunded)
}

func (p *Payment) transition(from, to PaymentStatus) error {
	if p.Status != from {
		return &PaymentTransitionError{From: p.Status, To: to}
	}
	p.Status = to
	p.UpdatedAt = time.Now().UTC()
	return nil
}

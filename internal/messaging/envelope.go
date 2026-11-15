// Package messaging define o formato das mensagens trocadas com o
// orquestrador da Saga (ADR-006) e a publicação no SQS.
package messaging

import (
	"encoding/json"
	"errors"
	"time"

	"billing-autoshop/internal/domain"

	"github.com/google/uuid"
)

// Comandos recebidos do orquestrador
const (
	CmdGenerateBudget = "GenerateBudget"
	CmdCancelBudget   = "CancelBudget"
	CmdRequestPayment = "RequestPayment"
	CmdRefundPayment  = "RefundPayment"
)

// Eventos enviados ao orquestrador
const (
	EvtBudgetGenerated        = "BudgetGenerated"
	EvtBudgetGenerationFailed = "BudgetGenerationFailed"
	EvtBudgetCancelled        = "BudgetCancelled"
	EvtPaymentRequested       = "PaymentRequested"
	EvtPaymentConfirmed       = "PaymentConfirmed"
	EvtPaymentFailed          = "PaymentFailed"
	EvtPaymentRefunded        = "PaymentRefunded"
)

var ErrEmptyPayload = errors.New("mensagem sem payload")

// Envelope é o formato comum de todas as mensagens da Saga.
type Envelope struct {
	MessageID     string          `json:"message_id"`
	Type          string          `json:"type"`
	SagaID        string          `json:"saga_id"`
	OrderID       string          `json:"order_id"`
	CorrelationID string          `json:"correlation_id"`
	OccurredAt    string          `json:"occurred_at"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

type GenerateBudgetPayload struct {
	Services []domain.ServiceInput `json:"services"`
	Parts    []domain.PartInput    `json:"parts"`
}

type BudgetGeneratedPayload struct {
	BudgetID      string  `json:"budget_id"`
	TotalServices float64 `json:"total_services"`
	TotalParts    float64 `json:"total_parts"`
	Total         float64 `json:"total"`
}

type FailurePayload struct {
	Reason string `json:"reason"`
}

type PaymentRequestedPayload struct {
	CheckoutURL string  `json:"checkout_url"`
	Amount      float64 `json:"amount"`
}

type PaymentFailedPayload struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// NewEvent monta um evento com ID e data próprios, mantendo a Saga, a OS e
// o correlation_id de origem.
func NewEvent(eventType, sagaID, orderID, correlationID string, payload any) (Envelope, error) {
	env := Envelope{
		MessageID:     uuid.NewString(),
		Type:          eventType,
		SagaID:        sagaID,
		OrderID:       orderID,
		CorrelationID: correlationID,
		OccurredAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return Envelope{}, err
		}
		env.Payload = raw
	}
	return env, nil
}

// ReplyTo monta o evento de resposta a um comando recebido.
func ReplyTo(cmd Envelope, eventType string, payload any) (Envelope, error) {
	return NewEvent(eventType, cmd.SagaID, cmd.OrderID, cmd.CorrelationID, payload)
}

func (e Envelope) DecodePayload(v any) error {
	if len(e.Payload) == 0 {
		return ErrEmptyPayload
	}
	return json.Unmarshal(e.Payload, v)
}

package notifier

import (
	"context"

	"billing-autoshop/internal/domain"
	"billing-autoshop/internal/messaging"
	"billing-autoshop/pkg/logger"
)

// Publisher envia eventos para a fila do orquestrador.
type Publisher interface {
	Publish(ctx context.Context, env messaging.Envelope) error
}

// SagaNotifier publica o resultado dos pagamentos na fila os-events, com a
// Saga e o correlation_id guardados quando o pagamento foi solicitado.
type SagaNotifier struct {
	publisher Publisher
}

func NewSagaNotifier(p Publisher) *SagaNotifier {
	return &SagaNotifier{publisher: p}
}

func (n *SagaNotifier) PaymentConfirmed(p domain.Payment) {
	n.publish(p, messaging.EvtPaymentConfirmed, nil)
}

func (n *SagaNotifier) PaymentFailed(p domain.Payment) {
	n.publish(p, messaging.EvtPaymentFailed, messaging.PaymentFailedPayload{Status: string(p.Status), Reason: p.FailureReason})
}

// publish registra no log se o envio falhar após as tentativas do
// publicador; nesse caso, o prazo do passo no orquestrador trata o caso.
func (n *SagaNotifier) publish(p domain.Payment, eventType string, payload any) {
	env, err := messaging.NewEvent(eventType, p.SagaID, p.OrderID, p.CorrelationID, payload)
	if err == nil {
		err = n.publisher.Publish(context.Background(), env)
	}
	if err != nil {
		logger.Error("falha ao publicar evento", "type", eventType, "order_id", p.OrderID, "correlation_id", p.CorrelationID, "error", err.Error())
		return
	}
	logger.Info("evento publicado", "type", eventType, "order_id", p.OrderID, "correlation_id", p.CorrelationID)
}

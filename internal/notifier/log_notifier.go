// Package notifier avisa o orquestrador da Saga sobre o resultado dos
// pagamentos.
package notifier

import (
	"billing-autoshop/internal/domain"
	"billing-autoshop/pkg/logger"
)

// LogNotifier apenas registra o resultado no log. É substituído pelo envio
// para a fila os-events na integração com a Saga.
type LogNotifier struct{}

func (LogNotifier) PaymentConfirmed(p domain.Payment) {
	logger.Info("pagamento confirmado", "order_id", p.OrderID, "correlation_id", p.CorrelationID)
}

func (LogNotifier) PaymentFailed(p domain.Payment) {
	logger.Info("pagamento não concluído", "order_id", p.OrderID, "status", p.Status, "reason", p.FailureReason, "correlation_id", p.CorrelationID)
}

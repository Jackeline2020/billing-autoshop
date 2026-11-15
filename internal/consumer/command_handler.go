// Package consumer processa os comandos que o orquestrador da Saga envia
// para a fila billing-commands.
package consumer

import (
	"context"
	"errors"
	"fmt"

	"billing-autoshop/internal/domain"
	"billing-autoshop/internal/messaging"
	"billing-autoshop/internal/notifier"
	"billing-autoshop/internal/usecase"
)

// ErrPermanent marca falhas que não se resolvem com nova tentativa. A
// mensagem é descartada e o prazo do passo no orquestrador trata o caso.
var ErrPermanent = errors.New("falha permanente")

type CommandHandler struct {
	budgets   *usecase.BudgetUseCase
	payments  *usecase.PaymentUseCase
	publisher notifier.Publisher
}

func NewCommandHandler(b *usecase.BudgetUseCase, p *usecase.PaymentUseCase, pub notifier.Publisher) *CommandHandler {
	return &CommandHandler{budgets: b, payments: p, publisher: pub}
}

// Handle executa o comando. Falhas de negócio viram eventos de falha para o
// orquestrador; falhas técnicas fazem a mensagem ser tentada de novo.
func (h *CommandHandler) Handle(ctx context.Context, cmd messaging.Envelope) error {
	switch cmd.Type {
	case messaging.CmdGenerateBudget:
		return h.generateBudget(ctx, cmd)

	case messaging.CmdCancelBudget:
		_, err := h.budgets.Cancel(cmd.OrderID)
		if err != nil && !errors.Is(err, domain.ErrBudgetNotFound) {
			return err
		}
		return h.reply(ctx, cmd, messaging.EvtBudgetCancelled, nil)

	case messaging.CmdRequestPayment:
		return h.requestPayment(ctx, cmd)

	case messaging.CmdRefundPayment:
		_, err := h.payments.Refund(cmd.OrderID)
		if err != nil && !errors.Is(err, domain.ErrPaymentNotFound) {
			return err
		}
		return h.reply(ctx, cmd, messaging.EvtPaymentRefunded, nil)
	}

	return fmt.Errorf("%w: comando desconhecido %q", ErrPermanent, cmd.Type)
}

func (h *CommandHandler) generateBudget(ctx context.Context, cmd messaging.Envelope) error {
	var payload messaging.GenerateBudgetPayload
	if err := cmd.DecodePayload(&payload); err != nil {
		return h.reply(ctx, cmd, messaging.EvtBudgetGenerationFailed, messaging.FailurePayload{Reason: "comando inválido: " + err.Error()})
	}

	b, err := h.budgets.Generate(cmd.OrderID, cmd.SagaID, cmd.CorrelationID, payload.Services, payload.Parts)
	if errors.Is(err, domain.ErrInvalidBudget) {
		return h.reply(ctx, cmd, messaging.EvtBudgetGenerationFailed, messaging.FailurePayload{Reason: err.Error()})
	}
	if err != nil {
		return err
	}
	return h.reply(ctx, cmd, messaging.EvtBudgetGenerated, messaging.BudgetGeneratedPayload{
		BudgetID: b.ID, TotalServices: b.TotalServices, TotalParts: b.TotalParts, Total: b.Total,
	})
}

// requestPayment responde com o link de pagamento. O resultado (aprovado,
// recusado ou expirado) chega depois, pelo webhook ou pela expiração.
func (h *CommandHandler) requestPayment(ctx context.Context, cmd messaging.Envelope) error {
	p, err := h.payments.Request(cmd.OrderID)
	if errors.Is(err, domain.ErrBudgetNotFound) || errors.Is(err, domain.ErrBudgetCancelled) {
		return h.reply(ctx, cmd, messaging.EvtPaymentFailed, messaging.PaymentFailedPayload{Reason: err.Error()})
	}
	if err != nil {
		return err
	}
	return h.reply(ctx, cmd, messaging.EvtPaymentRequested, messaging.PaymentRequestedPayload{CheckoutURL: p.CheckoutURL, Amount: p.Amount})
}

func (h *CommandHandler) reply(ctx context.Context, cmd messaging.Envelope, eventType string, payload any) error {
	evt, err := messaging.ReplyTo(cmd, eventType, payload)
	if err != nil {
		return err
	}
	return h.publisher.Publish(ctx, evt)
}

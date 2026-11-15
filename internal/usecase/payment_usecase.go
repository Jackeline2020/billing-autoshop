package usecase

import (
	"context"
	"errors"
	"time"

	"billing-autoshop/internal/domain"
	"billing-autoshop/internal/gateway"
	"billing-autoshop/internal/repository"
	"billing-autoshop/pkg/logger"

	"github.com/google/uuid"
)

// PaymentGateway é o provedor de pagamento (Mercado Pago ou mock).
type PaymentGateway interface {
	CreateCheckout(ctx context.Context, req gateway.CheckoutRequest) (gateway.Checkout, error)
	GetPayment(ctx context.Context, providerPaymentID string) (gateway.ProviderPayment, error)
	Refund(ctx context.Context, providerPaymentID string) error
}

// PaymentNotifier avisa o orquestrador da Saga sobre o resultado do
// pagamento, que chega de forma assíncrona (webhook ou expiração).
type PaymentNotifier interface {
	PaymentConfirmed(p domain.Payment)
	PaymentFailed(p domain.Payment)
}

type PaymentConfig struct {
	NotificationURL string        // endereço do webhook do Billing
	Timeout         time.Duration // prazo para o cliente pagar
}

type PaymentUseCase struct {
	payments repository.PaymentRepository
	budgets  *BudgetUseCase
	gateway  PaymentGateway
	notifier PaymentNotifier
	cfg      PaymentConfig
}

func NewPaymentUseCase(p repository.PaymentRepository, b *BudgetUseCase, g PaymentGateway, n PaymentNotifier, cfg PaymentConfig) *PaymentUseCase {
	return &PaymentUseCase{payments: p, budgets: b, gateway: g, notifier: n, cfg: cfg}
}

// Request aprova o orçamento e cria o link de pagamento da OS. Se a OS já
// tiver pagamento (comando repetido), devolve o existente.
func (u *PaymentUseCase) Request(orderID string) (domain.Payment, error) {
	existing, err := u.payments.FindByOrderID(orderID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, domain.ErrPaymentNotFound) {
		return domain.Payment{}, err
	}

	budget, err := u.budgets.Approve(orderID)
	var transErr *domain.InvalidTransitionError
	if errors.As(err, &transErr) {
		return domain.Payment{}, domain.ErrBudgetCancelled
	}
	if err != nil {
		return domain.Payment{}, err
	}

	checkout, err := u.gateway.CreateCheckout(context.Background(), gateway.CheckoutRequest{
		OrderID:         orderID,
		Description:     "Ordem de serviço " + orderID,
		Amount:          budget.Total,
		NotificationURL: u.cfg.NotificationURL,
	})
	if err != nil {
		return domain.Payment{}, err
	}

	p := domain.NewPayment(uuid.NewString(), budget, checkout.PreferenceID, checkout.URL, u.cfg.Timeout)
	err = u.payments.Create(p)
	if errors.Is(err, domain.ErrPaymentExists) {
		return u.payments.FindByOrderID(orderID)
	}
	if err != nil {
		return domain.Payment{}, err
	}
	logger.Info("link de pagamento criado", "order_id", orderID, "amount", p.Amount, "correlation_id", p.CorrelationID)
	return p, nil
}

// HandleNotification trata o aviso do provedor. O conteúdo do aviso não é
// usado como verdade: o status é sempre consultado no provedor.
func (u *PaymentUseCase) HandleNotification(providerPaymentID string) error {
	ctx := context.Background()
	pp, err := u.gateway.GetPayment(ctx, providerPaymentID)
	if errors.Is(err, gateway.ErrProviderNotFound) {
		return domain.ErrPaymentNotFound
	}
	if err != nil {
		return err
	}

	p, err := u.payments.FindByOrderID(pp.ExternalReference)
	if err != nil {
		return err
	}

	if p.IsFinal() {
		// Pago depois de a OS ter sido encerrada (prazo expirado): devolve o
		// dinheiro ao cliente.
		if p.Status == domain.PaymentExpired && pp.Status == gateway.ProviderApproved {
			logger.Warn("pagamento aprovado após expiração; estornando", "order_id", p.OrderID, "provider_payment_id", pp.ID)
			return u.gateway.Refund(ctx, pp.ID)
		}
		return nil
	}

	previous := p.Status
	switch pp.Status {
	case gateway.ProviderApproved:
		err = p.Approve(pp.ID)
	case gateway.ProviderRejected, gateway.ProviderCancelled:
		err = p.Reject(pp.ID, pp.StatusDetail)
	default:
		return nil // ainda em análise no provedor
	}
	if err != nil {
		return err
	}
	if err := u.payments.Update(p, previous); err != nil {
		return err
	}

	logger.Info("pagamento atualizado", "order_id", p.OrderID, "status", p.Status, "correlation_id", p.CorrelationID)
	if p.Status == domain.PaymentApproved {
		u.notifier.PaymentConfirmed(p)
	} else {
		u.notifier.PaymentFailed(p)
	}
	return nil
}

// ExpirePending encerra os pagamentos pendentes cujo prazo passou.
func (u *PaymentUseCase) ExpirePending(now time.Time) (int, error) {
	expired, err := u.payments.FindExpired(now)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, p := range expired {
		previous := p.Status
		if err := p.Expire(); err != nil {
			continue
		}
		if err := u.payments.Update(p, previous); err != nil {
			// o webhook pode ter atualizado o pagamento nesse meio-tempo
			logger.Warn("não foi possível expirar o pagamento", "order_id", p.OrderID, "error", err.Error())
			continue
		}
		logger.Info("pagamento expirado", "order_id", p.OrderID, "correlation_id", p.CorrelationID)
		u.notifier.PaymentFailed(p)
		count++
	}
	return count, nil
}

// Refund é a compensação do pagamento. Só estorna pagamentos aprovados;
// estornar de novo não faz nada. Um pagamento ainda pendente é encerrado,
// para que não seja aprovado depois da compensação.
func (u *PaymentUseCase) Refund(orderID string) (domain.Payment, error) {
	p, err := u.payments.FindByOrderID(orderID)
	if err != nil {
		return domain.Payment{}, err
	}

	previous := p.Status
	switch p.Status {
	case domain.PaymentApproved:
		if err := u.gateway.Refund(context.Background(), p.ProviderPaymentID); err != nil {
			return domain.Payment{}, err
		}
		_ = p.Refund()
	case domain.PaymentPending:
		_ = p.Expire()
	default:
		return p, nil
	}

	if err := u.payments.Update(p, previous); err != nil {
		return domain.Payment{}, err
	}
	logger.Info("pagamento compensado", "order_id", p.OrderID, "status", p.Status, "correlation_id", p.CorrelationID)
	return p, nil
}

func (u *PaymentUseCase) GetByOrderID(orderID string) (domain.Payment, error) {
	return u.payments.FindByOrderID(orderID)
}

package consumer_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"billing-autoshop/internal/consumer"
	"billing-autoshop/internal/domain"
	"billing-autoshop/internal/gateway"
	"billing-autoshop/internal/messaging"
	"billing-autoshop/internal/mocks"
	"billing-autoshop/internal/usecase"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type handlerMocks struct {
	budgets  *mocks.BudgetRepositoryMock
	payments *mocks.PaymentRepositoryMock
	gateway  *mocks.GatewayMock
	pub      *mocks.PublisherMock
}

func setupHandler() (*consumer.CommandHandler, handlerMocks) {
	m := handlerMocks{
		budgets:  new(mocks.BudgetRepositoryMock),
		payments: new(mocks.PaymentRepositoryMock),
		gateway:  new(mocks.GatewayMock),
		pub:      new(mocks.PublisherMock),
	}
	budgetUC := usecase.NewBudgetUseCase(m.budgets)
	paymentUC := usecase.NewPaymentUseCase(m.payments, budgetUC, m.gateway, &mocks.NotifierMock{}, usecase.PaymentConfig{Timeout: time.Minute})
	return consumer.NewCommandHandler(budgetUC, paymentUC, m.pub), m
}

func command(t *testing.T, cmdType string, payload any) messaging.Envelope {
	t.Helper()
	env, err := messaging.NewEvent(cmdType, "saga-1", "os-1", "corr-1", payload)
	require.NoError(t, err)
	return env
}

var budgetPayload = messaging.GenerateBudgetPayload{
	Services: []domain.ServiceInput{{ID: "s1", Name: "Troca de óleo", Price: 80}},
	Parts:    []domain.PartInput{{ID: "p1", Name: "Filtro", Quantity: 2, UnitPrice: 35}},
}

func TestHandle_GenerateBudget(t *testing.T) {
	h, m := setupHandler()
	m.budgets.On("Create", mock.Anything).Return(nil)
	m.pub.On("Publish", messaging.EvtBudgetGenerated, "os-1").Return(nil)

	require.NoError(t, h.Handle(context.Background(), command(t, messaging.CmdGenerateBudget, budgetPayload)))

	var p messaging.BudgetGeneratedPayload
	require.NoError(t, m.pub.Published[0].DecodePayload(&p))
	assert.Equal(t, 150.0, p.Total)
	assert.Equal(t, "saga-1", m.pub.Published[0].SagaID)
}

func TestHandle_GenerateBudgetInvalid(t *testing.T) {
	h, m := setupHandler()
	m.pub.On("Publish", messaging.EvtBudgetGenerationFailed, "os-1").Return(nil)

	require.NoError(t, h.Handle(context.Background(), command(t, messaging.CmdGenerateBudget, nil)))
	require.NoError(t, h.Handle(context.Background(), command(t, messaging.CmdGenerateBudget, messaging.GenerateBudgetPayload{})))
	assert.Len(t, m.pub.Published, 2)
	m.budgets.AssertNotCalled(t, "Create", mock.Anything)
}

func TestHandle_GenerateBudgetTechnicalError(t *testing.T) {
	h, m := setupHandler()
	m.budgets.On("Create", mock.Anything).Return(errors.New("banco fora"))

	err := h.Handle(context.Background(), command(t, messaging.CmdGenerateBudget, budgetPayload))
	assert.Error(t, err)
	assert.NotErrorIs(t, err, consumer.ErrPermanent)
	assert.Empty(t, m.pub.Published)
}

func TestHandle_CancelBudget(t *testing.T) {
	h, m := setupHandler()
	m.budgets.On("FindByOrderID", "os-1").Return(domain.Budget{ID: "b1", OrderID: "os-1", Status: domain.BudgetGenerated}, nil)
	m.budgets.On("UpdateStatus", mock.Anything, domain.BudgetGenerated).Return(nil)
	m.pub.On("Publish", messaging.EvtBudgetCancelled, "os-1").Return(nil)

	require.NoError(t, h.Handle(context.Background(), command(t, messaging.CmdCancelBudget, nil)))
	m.pub.AssertExpectations(t)
}

func TestHandle_CancelBudgetWithoutBudget(t *testing.T) {
	h, m := setupHandler()
	m.budgets.On("FindByOrderID", "os-1").Return(domain.Budget{}, domain.ErrBudgetNotFound)
	m.pub.On("Publish", messaging.EvtBudgetCancelled, "os-1").Return(nil)

	require.NoError(t, h.Handle(context.Background(), command(t, messaging.CmdCancelBudget, nil)))
}

func TestHandle_CancelBudgetError(t *testing.T) {
	h, m := setupHandler()
	m.budgets.On("FindByOrderID", "os-1").Return(domain.Budget{}, errors.New("banco fora"))
	assert.Error(t, h.Handle(context.Background(), command(t, messaging.CmdCancelBudget, nil)))
}

func TestHandle_RequestPayment(t *testing.T) {
	h, m := setupHandler()
	m.payments.On("FindByOrderID", "os-1").Return(domain.Payment{}, domain.ErrPaymentNotFound)
	m.budgets.On("FindByOrderID", "os-1").Return(domain.Budget{ID: "b1", OrderID: "os-1", Status: domain.BudgetApproved, Total: 150}, nil)
	m.gateway.On("CreateCheckout", mock.Anything).Return(gateway.Checkout{PreferenceID: "pref-1", URL: "http://checkout/pref-1"}, nil)
	m.payments.On("Create", mock.Anything).Return(nil)
	m.pub.On("Publish", messaging.EvtPaymentRequested, "os-1").Return(nil)

	require.NoError(t, h.Handle(context.Background(), command(t, messaging.CmdRequestPayment, nil)))
	var p messaging.PaymentRequestedPayload
	require.NoError(t, m.pub.Published[0].DecodePayload(&p))
	assert.Equal(t, "http://checkout/pref-1", p.CheckoutURL)
	assert.Equal(t, 150.0, p.Amount)
}

func TestHandle_RequestPaymentBudgetCancelled(t *testing.T) {
	h, m := setupHandler()
	m.payments.On("FindByOrderID", "os-1").Return(domain.Payment{}, domain.ErrPaymentNotFound)
	m.budgets.On("FindByOrderID", "os-1").Return(domain.Budget{OrderID: "os-1", Status: domain.BudgetCancelled}, nil)
	m.pub.On("Publish", messaging.EvtPaymentFailed, "os-1").Return(nil)

	require.NoError(t, h.Handle(context.Background(), command(t, messaging.CmdRequestPayment, nil)))
	m.pub.AssertExpectations(t)
}

func TestHandle_RequestPaymentProviderDown(t *testing.T) {
	h, m := setupHandler()
	m.payments.On("FindByOrderID", "os-1").Return(domain.Payment{}, domain.ErrPaymentNotFound)
	m.budgets.On("FindByOrderID", "os-1").Return(domain.Budget{OrderID: "os-1", Status: domain.BudgetApproved}, nil)
	m.gateway.On("CreateCheckout", mock.Anything).Return(gateway.Checkout{}, errors.New("timeout"))

	err := h.Handle(context.Background(), command(t, messaging.CmdRequestPayment, nil))
	assert.Error(t, err)
	assert.Empty(t, m.pub.Published)
}

func TestHandle_RefundPayment(t *testing.T) {
	h, m := setupHandler()
	m.payments.On("FindByOrderID", "os-1").Return(domain.Payment{ID: "p1", OrderID: "os-1", Status: domain.PaymentApproved, ProviderPaymentID: "mp-1"}, nil)
	m.gateway.On("Refund", "mp-1").Return(nil)
	m.payments.On("Update", mock.Anything, domain.PaymentApproved).Return(nil)
	m.pub.On("Publish", messaging.EvtPaymentRefunded, "os-1").Return(nil)

	require.NoError(t, h.Handle(context.Background(), command(t, messaging.CmdRefundPayment, nil)))
	m.pub.AssertExpectations(t)
}

func TestHandle_RefundWithoutPayment(t *testing.T) {
	h, m := setupHandler()
	m.payments.On("FindByOrderID", "os-1").Return(domain.Payment{}, domain.ErrPaymentNotFound)
	m.pub.On("Publish", messaging.EvtPaymentRefunded, "os-1").Return(nil)
	require.NoError(t, h.Handle(context.Background(), command(t, messaging.CmdRefundPayment, nil)))
}

func TestHandle_RefundError(t *testing.T) {
	h, m := setupHandler()
	m.payments.On("FindByOrderID", "os-1").Return(domain.Payment{}, errors.New("banco fora"))
	assert.Error(t, h.Handle(context.Background(), command(t, messaging.CmdRefundPayment, nil)))
}

func TestHandle_UnknownCommand(t *testing.T) {
	h, _ := setupHandler()
	assert.ErrorIs(t, h.Handle(context.Background(), command(t, "Desconhecido", nil)), consumer.ErrPermanent)
}

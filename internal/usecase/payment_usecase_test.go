package usecase_test

import (
	"errors"
	"testing"
	"time"

	"billing-autoshop/internal/domain"
	"billing-autoshop/internal/gateway"
	"billing-autoshop/internal/mocks"
	"billing-autoshop/internal/repository"
	"billing-autoshop/internal/usecase"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type paymentMocks struct {
	payments *mocks.PaymentRepositoryMock
	budgets  *mocks.BudgetRepositoryMock
	gateway  *mocks.GatewayMock
	notifier *mocks.NotifierMock
}

func setupPayment() (*usecase.PaymentUseCase, paymentMocks) {
	m := paymentMocks{
		payments: new(mocks.PaymentRepositoryMock),
		budgets:  new(mocks.BudgetRepositoryMock),
		gateway:  new(mocks.GatewayMock),
		notifier: &mocks.NotifierMock{},
	}
	cfg := usecase.PaymentConfig{NotificationURL: "http://billing/payments/webhook", Timeout: 15 * time.Minute}
	uc := usecase.NewPaymentUseCase(m.payments, usecase.NewBudgetUseCase(m.budgets), m.gateway, m.notifier, cfg)
	return uc, m
}

func paymentIn(status domain.PaymentStatus) domain.Payment {
	return domain.Payment{ID: "p1", OrderID: "os-1", Status: status, Amount: 80, ProviderPaymentID: "mp-1", CorrelationID: "corr-1"}
}

func TestRequest_CreatesCheckout(t *testing.T) {
	uc, m := setupPayment()
	m.payments.On("FindByOrderID", "os-1").Return(domain.Payment{}, domain.ErrPaymentNotFound)
	b := budgetIn(domain.BudgetGenerated)
	b.SagaID, b.CorrelationID = "saga-1", "corr-1"
	m.budgets.On("FindByOrderID", "os-1").Return(b, nil)
	m.budgets.On("UpdateStatus", mock.Anything, domain.BudgetGenerated).Return(nil)
	m.gateway.On("CreateCheckout", gateway.CheckoutRequest{
		OrderID: "os-1", Description: "Ordem de serviço os-1", Amount: 80, NotificationURL: "http://billing/payments/webhook",
	}).Return(gateway.Checkout{PreferenceID: "pref-1", URL: "http://checkout/pref-1"}, nil)
	m.payments.On("Create", mock.AnythingOfType("domain.Payment")).Return(nil)

	p, err := uc.Request("os-1")
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentPending, p.Status)
	assert.Equal(t, "http://checkout/pref-1", p.CheckoutURL)
	assert.Equal(t, "saga-1", p.SagaID)
	assert.Equal(t, 80.0, p.Amount)
}

func TestRequest_ExistingPaymentIsReturned(t *testing.T) {
	uc, m := setupPayment()
	m.payments.On("FindByOrderID", "os-1").Return(paymentIn(domain.PaymentPending), nil)

	p, err := uc.Request("os-1")
	require.NoError(t, err)
	assert.Equal(t, "p1", p.ID)
	m.gateway.AssertNotCalled(t, "CreateCheckout", mock.Anything)
}

func TestRequest_ConcurrentCreateReturnsExisting(t *testing.T) {
	uc, m := setupPayment()
	m.payments.On("FindByOrderID", "os-1").Return(domain.Payment{}, domain.ErrPaymentNotFound).Once()
	m.budgets.On("FindByOrderID", "os-1").Return(budgetIn(domain.BudgetApproved), nil)
	m.gateway.On("CreateCheckout", mock.Anything).Return(gateway.Checkout{PreferenceID: "pref-1"}, nil)
	m.payments.On("Create", mock.Anything).Return(domain.ErrPaymentExists)
	m.payments.On("FindByOrderID", "os-1").Return(paymentIn(domain.PaymentPending), nil).Once()

	p, err := uc.Request("os-1")
	require.NoError(t, err)
	assert.Equal(t, "p1", p.ID)
}

func TestRequest_Errors(t *testing.T) {
	t.Run("erro ao consultar pagamento", func(t *testing.T) {
		uc, m := setupPayment()
		m.payments.On("FindByOrderID", "os-1").Return(domain.Payment{}, errors.New("banco fora"))
		_, err := uc.Request("os-1")
		assert.Error(t, err)
	})
	t.Run("orçamento cancelado", func(t *testing.T) {
		uc, m := setupPayment()
		m.payments.On("FindByOrderID", "os-1").Return(domain.Payment{}, domain.ErrPaymentNotFound)
		m.budgets.On("FindByOrderID", "os-1").Return(budgetIn(domain.BudgetCancelled), nil)
		_, err := uc.Request("os-1")
		assert.ErrorIs(t, err, domain.ErrBudgetCancelled)
	})
	t.Run("orçamento inexistente", func(t *testing.T) {
		uc, m := setupPayment()
		m.payments.On("FindByOrderID", "os-1").Return(domain.Payment{}, domain.ErrPaymentNotFound)
		m.budgets.On("FindByOrderID", "os-1").Return(domain.Budget{}, domain.ErrBudgetNotFound)
		_, err := uc.Request("os-1")
		assert.ErrorIs(t, err, domain.ErrBudgetNotFound)
	})
	t.Run("provedor fora do ar", func(t *testing.T) {
		uc, m := setupPayment()
		m.payments.On("FindByOrderID", "os-1").Return(domain.Payment{}, domain.ErrPaymentNotFound)
		m.budgets.On("FindByOrderID", "os-1").Return(budgetIn(domain.BudgetApproved), nil)
		m.gateway.On("CreateCheckout", mock.Anything).Return(gateway.Checkout{}, errors.New("timeout"))
		_, err := uc.Request("os-1")
		assert.Error(t, err)
	})
	t.Run("erro ao gravar", func(t *testing.T) {
		uc, m := setupPayment()
		m.payments.On("FindByOrderID", "os-1").Return(domain.Payment{}, domain.ErrPaymentNotFound)
		m.budgets.On("FindByOrderID", "os-1").Return(budgetIn(domain.BudgetApproved), nil)
		m.gateway.On("CreateCheckout", mock.Anything).Return(gateway.Checkout{}, nil)
		m.payments.On("Create", mock.Anything).Return(errors.New("banco fora"))
		_, err := uc.Request("os-1")
		assert.Error(t, err)
	})
}

func TestHandleNotification_Approved(t *testing.T) {
	uc, m := setupPayment()
	m.gateway.On("GetPayment", "mp-1").Return(gateway.ProviderPayment{ID: "mp-1", Status: "approved", ExternalReference: "os-1"}, nil)
	m.payments.On("FindByOrderID", "os-1").Return(paymentIn(domain.PaymentPending), nil)
	m.payments.On("Update", mock.Anything, domain.PaymentPending).Return(nil)

	require.NoError(t, uc.HandleNotification("mp-1"))
	require.Len(t, m.notifier.Confirmed, 1)
	assert.Equal(t, domain.PaymentApproved, m.notifier.Confirmed[0].Status)
	assert.Empty(t, m.notifier.Failed)
}

func TestHandleNotification_Rejected(t *testing.T) {
	uc, m := setupPayment()
	m.gateway.On("GetPayment", "mp-1").Return(gateway.ProviderPayment{ID: "mp-1", Status: "rejected", StatusDetail: "cc_rejected_other_reason", ExternalReference: "os-1"}, nil)
	m.payments.On("FindByOrderID", "os-1").Return(paymentIn(domain.PaymentPending), nil)
	m.payments.On("Update", mock.Anything, domain.PaymentPending).Return(nil)

	require.NoError(t, uc.HandleNotification("mp-1"))
	require.Len(t, m.notifier.Failed, 1)
	assert.Equal(t, "cc_rejected_other_reason", m.notifier.Failed[0].FailureReason)
}

func TestHandleNotification_StillPending(t *testing.T) {
	uc, m := setupPayment()
	m.gateway.On("GetPayment", "mp-1").Return(gateway.ProviderPayment{ID: "mp-1", Status: "in_process", ExternalReference: "os-1"}, nil)
	m.payments.On("FindByOrderID", "os-1").Return(paymentIn(domain.PaymentPending), nil)

	require.NoError(t, uc.HandleNotification("mp-1"))
	m.payments.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestHandleNotification_AlreadyFinalIsIgnored(t *testing.T) {
	uc, m := setupPayment()
	m.gateway.On("GetPayment", "mp-1").Return(gateway.ProviderPayment{ID: "mp-1", Status: "approved", ExternalReference: "os-1"}, nil)
	m.payments.On("FindByOrderID", "os-1").Return(paymentIn(domain.PaymentApproved), nil)

	require.NoError(t, uc.HandleNotification("mp-1"))
	assert.Empty(t, m.notifier.Confirmed)
}

func TestHandleNotification_ApprovedAfterExpirationIsRefunded(t *testing.T) {
	uc, m := setupPayment()
	m.gateway.On("GetPayment", "mp-2").Return(gateway.ProviderPayment{ID: "mp-2", Status: "approved", ExternalReference: "os-1"}, nil)
	m.payments.On("FindByOrderID", "os-1").Return(paymentIn(domain.PaymentExpired), nil)
	m.gateway.On("Refund", "mp-2").Return(nil)

	require.NoError(t, uc.HandleNotification("mp-2"))
	m.gateway.AssertExpectations(t)
	assert.Empty(t, m.notifier.Confirmed)
}

func TestHandleNotification_Errors(t *testing.T) {
	t.Run("pagamento desconhecido no provedor", func(t *testing.T) {
		uc, m := setupPayment()
		m.gateway.On("GetPayment", "x").Return(gateway.ProviderPayment{}, gateway.ErrProviderNotFound)
		assert.ErrorIs(t, uc.HandleNotification("x"), domain.ErrPaymentNotFound)
	})
	t.Run("provedor fora do ar", func(t *testing.T) {
		uc, m := setupPayment()
		m.gateway.On("GetPayment", "x").Return(gateway.ProviderPayment{}, errors.New("timeout"))
		assert.Error(t, uc.HandleNotification("x"))
	})
	t.Run("OS sem pagamento", func(t *testing.T) {
		uc, m := setupPayment()
		m.gateway.On("GetPayment", "x").Return(gateway.ProviderPayment{ID: "x", Status: "approved", ExternalReference: "outra"}, nil)
		m.payments.On("FindByOrderID", "outra").Return(domain.Payment{}, domain.ErrPaymentNotFound)
		assert.ErrorIs(t, uc.HandleNotification("x"), domain.ErrPaymentNotFound)
	})
	t.Run("conflito ao gravar", func(t *testing.T) {
		uc, m := setupPayment()
		m.gateway.On("GetPayment", "mp-1").Return(gateway.ProviderPayment{ID: "mp-1", Status: "approved", ExternalReference: "os-1"}, nil)
		m.payments.On("FindByOrderID", "os-1").Return(paymentIn(domain.PaymentPending), nil)
		m.payments.On("Update", mock.Anything, mock.Anything).Return(repository.ErrConcurrentUpdate)
		assert.ErrorIs(t, uc.HandleNotification("mp-1"), repository.ErrConcurrentUpdate)
		assert.Empty(t, m.notifier.Confirmed)
	})
}

func TestExpirePending(t *testing.T) {
	uc, m := setupPayment()
	now := time.Now()
	first, second, third := paymentIn(domain.PaymentPending), paymentIn(domain.PaymentPending), paymentIn(domain.PaymentApproved)
	first.ID, second.ID, third.ID = "a", "b", "c"
	m.payments.On("FindExpired", now).Return([]domain.Payment{first, second, third}, nil)
	m.payments.On("Update", mock.MatchedBy(func(p domain.Payment) bool { return p.ID == "a" }), domain.PaymentPending).Return(nil)
	m.payments.On("Update", mock.MatchedBy(func(p domain.Payment) bool { return p.ID == "b" }), domain.PaymentPending).Return(repository.ErrConcurrentUpdate)

	count, err := uc.ExpirePending(now)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	require.Len(t, m.notifier.Failed, 1)
	assert.Equal(t, domain.PaymentExpired, m.notifier.Failed[0].Status)
}

func TestExpirePending_Error(t *testing.T) {
	uc, m := setupPayment()
	m.payments.On("FindExpired", mock.Anything).Return([]domain.Payment(nil), errors.New("banco fora"))
	_, err := uc.ExpirePending(time.Now())
	assert.Error(t, err)
}

func TestRefund_Approved(t *testing.T) {
	uc, m := setupPayment()
	m.payments.On("FindByOrderID", "os-1").Return(paymentIn(domain.PaymentApproved), nil)
	m.gateway.On("Refund", "mp-1").Return(nil)
	m.payments.On("Update", mock.Anything, domain.PaymentApproved).Return(nil)

	p, err := uc.Refund("os-1")
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentRefunded, p.Status)
}

func TestRefund_PendingIsClosed(t *testing.T) {
	uc, m := setupPayment()
	m.payments.On("FindByOrderID", "os-1").Return(paymentIn(domain.PaymentPending), nil)
	m.payments.On("Update", mock.Anything, domain.PaymentPending).Return(nil)

	p, err := uc.Refund("os-1")
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentExpired, p.Status)
	m.gateway.AssertNotCalled(t, "Refund", mock.Anything)
}

func TestRefund_NothingToRefund(t *testing.T) {
	for _, status := range []domain.PaymentStatus{domain.PaymentRefunded, domain.PaymentRejected, domain.PaymentExpired} {
		uc, m := setupPayment()
		m.payments.On("FindByOrderID", "os-1").Return(paymentIn(status), nil)

		p, err := uc.Refund("os-1")
		require.NoError(t, err)
		assert.Equal(t, status, p.Status)
		m.payments.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	}
}

func TestRefund_Errors(t *testing.T) {
	t.Run("sem pagamento", func(t *testing.T) {
		uc, m := setupPayment()
		m.payments.On("FindByOrderID", "x").Return(domain.Payment{}, domain.ErrPaymentNotFound)
		_, err := uc.Refund("x")
		assert.ErrorIs(t, err, domain.ErrPaymentNotFound)
	})
	t.Run("provedor recusa", func(t *testing.T) {
		uc, m := setupPayment()
		m.payments.On("FindByOrderID", "os-1").Return(paymentIn(domain.PaymentApproved), nil)
		m.gateway.On("Refund", "mp-1").Return(errors.New("timeout"))
		_, err := uc.Refund("os-1")
		assert.Error(t, err)
		m.payments.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	})
	t.Run("erro ao gravar", func(t *testing.T) {
		uc, m := setupPayment()
		m.payments.On("FindByOrderID", "os-1").Return(paymentIn(domain.PaymentPending), nil)
		m.payments.On("Update", mock.Anything, mock.Anything).Return(errors.New("banco fora"))
		_, err := uc.Refund("os-1")
		assert.Error(t, err)
	})
}

func TestPaymentGetByOrderID(t *testing.T) {
	uc, m := setupPayment()
	m.payments.On("FindByOrderID", "os-1").Return(paymentIn(domain.PaymentPending), nil)
	p, err := uc.GetByOrderID("os-1")
	require.NoError(t, err)
	assert.Equal(t, "p1", p.ID)
}

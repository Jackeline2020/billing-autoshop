package domain_test

import (
	"testing"
	"time"

	"billing-autoshop/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPayment(t *testing.T) domain.Payment {
	t.Helper()
	b, err := domain.NewBudget("b1", "os-1", services, nil)
	require.NoError(t, err)
	b.SagaID, b.CorrelationID = "saga-1", "corr-1"
	return domain.NewPayment("p1", b, "pref-1", "http://checkout/pref-1", 15*time.Minute)
}

func TestNewPayment(t *testing.T) {
	p := newPayment(t)
	assert.Equal(t, domain.PaymentPending, p.Status)
	assert.Equal(t, 200.5, p.Amount)
	assert.Equal(t, "os-1", p.OrderID)
	assert.Equal(t, "b1", p.BudgetID)
	assert.Equal(t, "saga-1", p.SagaID)
	assert.Equal(t, "corr-1", p.CorrelationID)
	assert.WithinDuration(t, time.Now().Add(15*time.Minute), p.ExpiresAt, time.Minute)
	assert.False(t, p.IsFinal())
}

func TestPayment_ApproveAndRefund(t *testing.T) {
	p := newPayment(t)
	require.NoError(t, p.Approve("mp-123"))
	assert.Equal(t, domain.PaymentApproved, p.Status)
	assert.Equal(t, "mp-123", p.ProviderPaymentID)
	assert.True(t, p.IsFinal())

	require.NoError(t, p.Refund())
	assert.Equal(t, domain.PaymentRefunded, p.Status)
	assert.Error(t, p.Refund())
}

func TestPayment_Reject(t *testing.T) {
	p := newPayment(t)
	require.NoError(t, p.Reject("mp-9", "cc_rejected_other_reason"))
	assert.Equal(t, domain.PaymentRejected, p.Status)
	assert.Equal(t, "cc_rejected_other_reason", p.FailureReason)

	var transErr *domain.PaymentTransitionError
	assert.ErrorAs(t, p.Approve("mp-9"), &transErr)
	assert.Contains(t, transErr.Error(), "recusado")
	assert.Error(t, p.Refund())
}

func TestPayment_Expire(t *testing.T) {
	p := newPayment(t)
	require.NoError(t, p.Expire())
	assert.Equal(t, domain.PaymentExpired, p.Status)
	assert.NotEmpty(t, p.FailureReason)
	assert.Error(t, p.Expire())
}

package repository_test

import (
	"testing"
	"time"

	"billing-autoshop/internal/domain"
	"billing-autoshop/internal/repository"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStoredPayment(t *testing.T, timeout time.Duration) (domain.Payment, *repository.PaymentPostgresRepository) {
	t.Helper()
	b := newBudget(t)
	require.NoError(t, repository.NewBudgetPostgresRepository(pool).Create(b))
	p := domain.NewPayment(uuid.NewString(), b, "pref-1", "http://checkout/pref-1", timeout)
	repo := repository.NewPaymentPostgresRepository(pool)
	require.NoError(t, repo.Create(p))
	return p, repo
}

func TestPaymentRepository_CreateAndFind(t *testing.T) {
	requireDB(t)
	p, repo := newStoredPayment(t, time.Hour)

	found, err := repo.FindByOrderID(p.OrderID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, found.ID)
	assert.Equal(t, domain.PaymentPending, found.Status)
	assert.Equal(t, p.Amount, found.Amount)
	assert.Equal(t, "http://checkout/pref-1", found.CheckoutURL)
	assert.Equal(t, "pref-1", found.ProviderPreferenceID)
	assert.Equal(t, "saga-1", found.SagaID)
	assert.WithinDuration(t, p.ExpiresAt, found.ExpiresAt, time.Second)
}

func TestPaymentRepository_Duplicate(t *testing.T) {
	requireDB(t)
	p, repo := newStoredPayment(t, time.Hour)
	again := p
	again.ID = uuid.NewString()
	assert.ErrorIs(t, repo.Create(again), domain.ErrPaymentExists)
}

func TestPaymentRepository_NotFound(t *testing.T) {
	requireDB(t)
	_, err := repository.NewPaymentPostgresRepository(pool).FindByOrderID(uuid.NewString())
	assert.ErrorIs(t, err, domain.ErrPaymentNotFound)
}

func TestPaymentRepository_Update(t *testing.T) {
	requireDB(t)
	p, repo := newStoredPayment(t, time.Hour)

	stale := p
	require.NoError(t, p.Approve("mp-123"))
	require.NoError(t, repo.Update(p, domain.PaymentPending))

	require.NoError(t, stale.Expire())
	assert.ErrorIs(t, repo.Update(stale, domain.PaymentPending), repository.ErrConcurrentUpdate)

	found, err := repo.FindByOrderID(p.OrderID)
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentApproved, found.Status)
	assert.Equal(t, "mp-123", found.ProviderPaymentID)
}

func TestPaymentRepository_FindExpired(t *testing.T) {
	requireDB(t)
	expired, repo := newStoredPayment(t, -time.Minute)
	valid, _ := newStoredPayment(t, time.Hour)

	result, err := repo.FindExpired(time.Now())
	require.NoError(t, err)

	ids := map[string]bool{}
	for _, p := range result {
		ids[p.ID] = true
	}
	assert.True(t, ids[expired.ID])
	assert.False(t, ids[valid.ID])
}

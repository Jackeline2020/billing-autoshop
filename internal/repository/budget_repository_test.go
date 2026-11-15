package repository_test

import (
	"testing"

	"billing-autoshop/internal/domain"
	"billing-autoshop/internal/repository"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBudget(t *testing.T) domain.Budget {
	t.Helper()
	b, err := domain.NewBudget(uuid.NewString(), uuid.NewString(),
		[]domain.ServiceInput{{ID: "s1", Name: "Troca de óleo", Price: 80}},
		[]domain.PartInput{{ID: "p1", Name: "Filtro", Quantity: 2, UnitPrice: 35.5}})
	require.NoError(t, err)
	b.SagaID = "saga-1"
	b.CorrelationID = "corr-1"
	return b
}

func TestBudgetRepository_CreateAndFind(t *testing.T) {
	requireDB(t)
	repo := repository.NewBudgetPostgresRepository(pool)
	b := newBudget(t)

	require.NoError(t, repo.Create(b))
	found, err := repo.FindByOrderID(b.OrderID)
	require.NoError(t, err)

	assert.Equal(t, b.ID, found.ID)
	assert.Equal(t, domain.BudgetGenerated, found.Status)
	assert.Equal(t, 151.0, found.Total)
	assert.Equal(t, 71.0, found.TotalParts)
	assert.Equal(t, "saga-1", found.SagaID)
	assert.Equal(t, "corr-1", found.CorrelationID)
	assert.Equal(t, b.Items, found.Items)
}

func TestBudgetRepository_DuplicateOrder(t *testing.T) {
	requireDB(t)
	repo := repository.NewBudgetPostgresRepository(pool)
	b := newBudget(t)
	require.NoError(t, repo.Create(b))

	again := newBudget(t)
	again.OrderID = b.OrderID
	assert.ErrorIs(t, repo.Create(again), domain.ErrBudgetExists)
}

func TestBudgetRepository_NotFound(t *testing.T) {
	requireDB(t)
	repo := repository.NewBudgetPostgresRepository(pool)
	_, err := repo.FindByOrderID(uuid.NewString())
	assert.ErrorIs(t, err, domain.ErrBudgetNotFound)
}

func TestBudgetRepository_UpdateStatus(t *testing.T) {
	requireDB(t)
	repo := repository.NewBudgetPostgresRepository(pool)
	b := newBudget(t)
	require.NoError(t, repo.Create(b))

	stale := b
	require.NoError(t, b.Approve())
	require.NoError(t, repo.UpdateStatus(b, domain.BudgetGenerated))

	// Uma segunda operação baseada na leitura antiga é recusada
	stale.Cancel()
	assert.ErrorIs(t, repo.UpdateStatus(stale, domain.BudgetGenerated), repository.ErrConcurrentUpdate)

	found, err := repo.FindByOrderID(b.OrderID)
	require.NoError(t, err)
	assert.Equal(t, domain.BudgetApproved, found.Status)
}

func TestBudgetRepository_WithoutSagaContext(t *testing.T) {
	requireDB(t)
	repo := repository.NewBudgetPostgresRepository(pool)
	b := newBudget(t)
	b.SagaID, b.CorrelationID = "", ""
	require.NoError(t, repo.Create(b))

	found, err := repo.FindByOrderID(b.OrderID)
	require.NoError(t, err)
	assert.Empty(t, found.SagaID)
}

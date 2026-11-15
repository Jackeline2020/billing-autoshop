package usecase_test

import (
	"errors"
	"testing"

	"billing-autoshop/internal/domain"
	"billing-autoshop/internal/mocks"
	"billing-autoshop/internal/usecase"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var services = []domain.ServiceInput{{ID: "s1", Name: "Troca de óleo", Price: 80}}

func budgetIn(status domain.BudgetStatus) domain.Budget {
	return domain.Budget{ID: "b1", OrderID: "os-1", Status: status, Total: 80}
}

func TestGenerate(t *testing.T) {
	repo := new(mocks.BudgetRepositoryMock)
	repo.On("Create", mock.MatchedBy(func(b domain.Budget) bool {
		return b.OrderID == "os-1" && b.SagaID == "saga-1" && b.CorrelationID == "corr-1" && b.ID != ""
	})).Return(nil)

	b, err := usecase.NewBudgetUseCase(repo).Generate("os-1", "saga-1", "corr-1", services, nil)
	require.NoError(t, err)
	assert.Equal(t, 80.0, b.Total)
}

func TestGenerate_RepeatedReturnsExisting(t *testing.T) {
	repo := new(mocks.BudgetRepositoryMock)
	existing := budgetIn(domain.BudgetGenerated)
	repo.On("Create", mock.Anything).Return(domain.ErrBudgetExists)
	repo.On("FindByOrderID", "os-1").Return(existing, nil)

	b, err := usecase.NewBudgetUseCase(repo).Generate("os-1", "s", "c", services, nil)
	require.NoError(t, err)
	assert.Equal(t, existing, b)
}

func TestGenerate_Errors(t *testing.T) {
	repo := new(mocks.BudgetRepositoryMock)
	uc := usecase.NewBudgetUseCase(repo)

	_, err := uc.Generate("os-1", "s", "c", nil, nil)
	assert.ErrorIs(t, err, domain.ErrInvalidBudget)

	repo.On("Create", mock.Anything).Return(errors.New("banco fora"))
	_, err = uc.Generate("os-1", "s", "c", services, nil)
	assert.Error(t, err)
}

func TestApprove(t *testing.T) {
	repo := new(mocks.BudgetRepositoryMock)
	repo.On("FindByOrderID", "os-1").Return(budgetIn(domain.BudgetGenerated), nil)
	repo.On("UpdateStatus", mock.Anything, domain.BudgetGenerated).Return(nil)

	b, err := usecase.NewBudgetUseCase(repo).Approve("os-1")
	require.NoError(t, err)
	assert.Equal(t, domain.BudgetApproved, b.Status)
}

func TestApprove_AlreadyApproved(t *testing.T) {
	repo := new(mocks.BudgetRepositoryMock)
	repo.On("FindByOrderID", "os-1").Return(budgetIn(domain.BudgetApproved), nil)

	_, err := usecase.NewBudgetUseCase(repo).Approve("os-1")
	require.NoError(t, err)
	repo.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything)
}

func TestApprove_Errors(t *testing.T) {
	repo := new(mocks.BudgetRepositoryMock)
	uc := usecase.NewBudgetUseCase(repo)

	repo.On("FindByOrderID", "x").Return(domain.Budget{}, domain.ErrBudgetNotFound)
	_, err := uc.Approve("x")
	assert.ErrorIs(t, err, domain.ErrBudgetNotFound)

	repo.On("FindByOrderID", "cancelado").Return(domain.Budget{OrderID: "cancelado", Status: domain.BudgetCancelled}, nil)
	var transErr *domain.InvalidTransitionError
	_, err = uc.Approve("cancelado")
	assert.ErrorAs(t, err, &transErr)

	repo.On("FindByOrderID", "os-1").Return(budgetIn(domain.BudgetGenerated), nil)
	repo.On("UpdateStatus", mock.Anything, mock.Anything).Return(errors.New("conflito"))
	_, err = uc.Approve("os-1")
	assert.Error(t, err)
}

func TestCancel(t *testing.T) {
	repo := new(mocks.BudgetRepositoryMock)
	repo.On("FindByOrderID", "os-1").Return(budgetIn(domain.BudgetApproved), nil)
	repo.On("UpdateStatus", mock.Anything, domain.BudgetApproved).Return(nil)

	b, err := usecase.NewBudgetUseCase(repo).Cancel("os-1")
	require.NoError(t, err)
	assert.Equal(t, domain.BudgetCancelled, b.Status)
}

func TestCancel_AlreadyCancelled(t *testing.T) {
	repo := new(mocks.BudgetRepositoryMock)
	repo.On("FindByOrderID", "os-1").Return(budgetIn(domain.BudgetCancelled), nil)

	_, err := usecase.NewBudgetUseCase(repo).Cancel("os-1")
	require.NoError(t, err)
	repo.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything)
}

func TestCancel_Errors(t *testing.T) {
	repo := new(mocks.BudgetRepositoryMock)
	uc := usecase.NewBudgetUseCase(repo)

	repo.On("FindByOrderID", "x").Return(domain.Budget{}, domain.ErrBudgetNotFound)
	_, err := uc.Cancel("x")
	assert.ErrorIs(t, err, domain.ErrBudgetNotFound)

	repo.On("FindByOrderID", "os-1").Return(budgetIn(domain.BudgetGenerated), nil)
	repo.On("UpdateStatus", mock.Anything, mock.Anything).Return(errors.New("conflito"))
	_, err = uc.Cancel("os-1")
	assert.Error(t, err)
}

func TestGetByOrderID(t *testing.T) {
	repo := new(mocks.BudgetRepositoryMock)
	repo.On("FindByOrderID", "os-1").Return(budgetIn(domain.BudgetGenerated), nil)

	b, err := usecase.NewBudgetUseCase(repo).GetByOrderID("os-1")
	require.NoError(t, err)
	assert.Equal(t, "b1", b.ID)
}

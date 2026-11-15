package mocks

import (
	"billing-autoshop/internal/domain"

	"github.com/stretchr/testify/mock"
)

type BudgetRepositoryMock struct {
	mock.Mock
}

func (m *BudgetRepositoryMock) Create(b domain.Budget) error {
	return m.Called(b).Error(0)
}

func (m *BudgetRepositoryMock) FindByOrderID(orderID string) (domain.Budget, error) {
	args := m.Called(orderID)
	return args.Get(0).(domain.Budget), args.Error(1)
}

func (m *BudgetRepositoryMock) UpdateStatus(b domain.Budget, previous domain.BudgetStatus) error {
	return m.Called(b, previous).Error(0)
}

package usecase

import (
	"errors"

	"billing-autoshop/internal/domain"
	"billing-autoshop/internal/repository"

	"github.com/google/uuid"
)

type BudgetUseCase struct {
	repo repository.BudgetRepository
}

func NewBudgetUseCase(r repository.BudgetRepository) *BudgetUseCase {
	return &BudgetUseCase{repo: r}
}

// Generate cria o orçamento da OS. Se ele já existir (comando repetido),
// devolve o existente.
func (u *BudgetUseCase) Generate(orderID, sagaID, correlationID string, services []domain.ServiceInput, parts []domain.PartInput) (domain.Budget, error) {
	b, err := domain.NewBudget(uuid.NewString(), orderID, services, parts)
	if err != nil {
		return domain.Budget{}, err
	}
	b.SagaID = sagaID
	b.CorrelationID = correlationID

	err = u.repo.Create(b)
	if errors.Is(err, domain.ErrBudgetExists) {
		return u.repo.FindByOrderID(orderID)
	}
	if err != nil {
		return domain.Budget{}, err
	}
	return b, nil
}

// Approve registra a aprovação do cliente (antes da cobrança).
func (u *BudgetUseCase) Approve(orderID string) (domain.Budget, error) {
	b, err := u.repo.FindByOrderID(orderID)
	if err != nil {
		return domain.Budget{}, err
	}
	previous := b.Status
	if err := b.Approve(); err != nil {
		return domain.Budget{}, err
	}
	if b.Status == previous {
		return b, nil
	}
	if err := u.repo.UpdateStatus(b, previous); err != nil {
		return domain.Budget{}, err
	}
	return b, nil
}

// Cancel é a compensação do orçamento. Cancelar um orçamento já cancelado
// não faz nada.
func (u *BudgetUseCase) Cancel(orderID string) (domain.Budget, error) {
	b, err := u.repo.FindByOrderID(orderID)
	if err != nil {
		return domain.Budget{}, err
	}
	previous := b.Status
	b.Cancel()
	if b.Status == previous {
		return b, nil
	}
	if err := u.repo.UpdateStatus(b, previous); err != nil {
		return domain.Budget{}, err
	}
	return b, nil
}

func (u *BudgetUseCase) GetByOrderID(orderID string) (domain.Budget, error) {
	return u.repo.FindByOrderID(orderID)
}

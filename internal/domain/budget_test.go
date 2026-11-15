package domain_test

import (
	"testing"

	"billing-autoshop/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var services = []domain.ServiceInput{{ID: "s1", Name: "Troca de óleo", Price: 80}, {ID: "s2", Name: "Alinhamento", Price: 120.5}}
var parts = []domain.PartInput{{ID: "p1", Name: "Óleo 5W30", Quantity: 4, UnitPrice: 45.9}}

func TestNewBudget_CalculatesTotals(t *testing.T) {
	b, err := domain.NewBudget("b1", "os-1", services, parts)
	require.NoError(t, err)

	assert.Equal(t, domain.BudgetGenerated, b.Status)
	assert.Equal(t, 200.5, b.TotalServices)
	assert.Equal(t, 183.6, b.TotalParts)
	assert.Equal(t, 384.1, b.Total)
	require.Len(t, b.Items, 3)
	assert.Equal(t, domain.ItemPart, b.Items[2].Kind)
	assert.Equal(t, 183.6, b.Items[2].Subtotal())
	assert.False(t, b.CreatedAt.IsZero())
}

func TestNewBudget_WithoutParts(t *testing.T) {
	b, err := domain.NewBudget("b1", "os-1", services[:1], nil)
	require.NoError(t, err)
	assert.Equal(t, 80.0, b.Total)
	assert.Zero(t, b.TotalParts)
}

func TestNewBudget_Invalid(t *testing.T) {
	cases := map[string]struct {
		orderID  string
		services []domain.ServiceInput
		parts    []domain.PartInput
	}{
		"sem OS":                 {"", services, nil},
		"sem serviços":           {"os-1", nil, parts},
		"serviço sem id":         {"os-1", []domain.ServiceInput{{Name: "x", Price: 1}}, nil},
		"serviço preço negativo": {"os-1", []domain.ServiceInput{{ID: "s", Name: "x", Price: -1}}, nil},
		"peça sem nome":          {"os-1", services, []domain.PartInput{{ID: "p", Quantity: 1, UnitPrice: 1}}},
		"peça quantidade zero":   {"os-1", services, []domain.PartInput{{ID: "p", Name: "x", Quantity: 0, UnitPrice: 1}}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := domain.NewBudget("b1", tc.orderID, tc.services, tc.parts)
			assert.ErrorIs(t, err, domain.ErrInvalidBudget)
		})
	}
}

func TestBudget_ApproveAndCancel(t *testing.T) {
	b, _ := domain.NewBudget("b1", "os-1", services, nil)

	require.NoError(t, b.Approve())
	assert.Equal(t, domain.BudgetApproved, b.Status)
	require.NoError(t, b.Approve()) // aprovar de novo não é erro

	b.Cancel()
	assert.Equal(t, domain.BudgetCancelled, b.Status)
	b.Cancel()
	assert.Equal(t, domain.BudgetCancelled, b.Status)

	var transErr *domain.InvalidTransitionError
	assert.ErrorAs(t, b.Approve(), &transErr)
	assert.Contains(t, transErr.Error(), "cancelado")
}

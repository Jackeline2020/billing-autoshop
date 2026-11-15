package domain

import (
	"errors"
	"fmt"
	"math"
	"time"
)

type BudgetStatus string

const (
	BudgetGenerated BudgetStatus = "gerado"
	BudgetApproved  BudgetStatus = "aprovado"
	BudgetCancelled BudgetStatus = "cancelado"
)

type ItemKind string

const (
	ItemService ItemKind = "servico"
	ItemPart    ItemKind = "peca"
)

var (
	ErrBudgetNotFound = errors.New("orçamento não encontrado")
	ErrBudgetExists   = errors.New("já existe orçamento para esta OS")
	ErrInvalidBudget  = errors.New("orçamento inválido")
)

// InvalidTransitionError indica uma mudança de status fora do fluxo
// (ex.: aprovar um orçamento cancelado).
type InvalidTransitionError struct {
	From, To BudgetStatus
}

func (e *InvalidTransitionError) Error() string {
	return fmt.Sprintf("transição de status inválida: %s → %s", e.From, e.To)
}

type BudgetItem struct {
	Kind        ItemKind `json:"kind"`
	ReferenceID string   `json:"reference_id"`
	Name        string   `json:"name"`
	Quantity    int      `json:"quantity"`
	UnitPrice   float64  `json:"unit_price"`
}

func (i BudgetItem) Subtotal() float64 {
	return roundMoney(i.UnitPrice * float64(i.Quantity))
}

// ServiceInput e PartInput são os dados recebidos do OS Service no comando
// de geração do orçamento.
type ServiceInput struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

type PartInput struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
}

type Budget struct {
	ID            string       `json:"id"`
	OrderID       string       `json:"order_id"`
	Status        BudgetStatus `json:"status"`
	Items         []BudgetItem `json:"items"`
	TotalServices float64      `json:"total_services"`
	TotalParts    float64      `json:"total_parts"`
	Total         float64      `json:"total"`
	SagaID        string       `json:"saga_id,omitempty"`
	CorrelationID string       `json:"correlation_id,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

// NewBudget monta o orçamento com as mesmas regras das fases anteriores:
// pelo menos um serviço, peças com quantidade positiva, total = serviços +
// (preço unitário × quantidade das peças).
func NewBudget(id, orderID string, services []ServiceInput, parts []PartInput) (Budget, error) {
	if orderID == "" {
		return Budget{}, fmt.Errorf("%w: OS é obrigatória", ErrInvalidBudget)
	}
	if len(services) == 0 {
		return Budget{}, fmt.Errorf("%w: pelo menos um serviço é obrigatório", ErrInvalidBudget)
	}

	b := Budget{ID: id, OrderID: orderID, Status: BudgetGenerated}
	for _, s := range services {
		if s.ID == "" || s.Name == "" || s.Price < 0 {
			return Budget{}, fmt.Errorf("%w: serviço sem identificação ou com preço negativo", ErrInvalidBudget)
		}
		b.Items = append(b.Items, BudgetItem{Kind: ItemService, ReferenceID: s.ID, Name: s.Name, Quantity: 1, UnitPrice: s.Price})
	}
	for _, p := range parts {
		if p.ID == "" || p.Name == "" || p.UnitPrice < 0 {
			return Budget{}, fmt.Errorf("%w: peça sem identificação ou com preço negativo", ErrInvalidBudget)
		}
		if p.Quantity <= 0 {
			return Budget{}, fmt.Errorf("%w: quantidade da peça deve ser maior que zero", ErrInvalidBudget)
		}
		b.Items = append(b.Items, BudgetItem{Kind: ItemPart, ReferenceID: p.ID, Name: p.Name, Quantity: p.Quantity, UnitPrice: p.UnitPrice})
	}

	b.calculateTotals()
	now := time.Now().UTC()
	b.CreatedAt, b.UpdatedAt = now, now
	return b, nil
}

// Approve registra que o cliente aprovou o orçamento.
func (b *Budget) Approve() error {
	if b.Status == BudgetApproved {
		return nil
	}
	if b.Status != BudgetGenerated {
		return &InvalidTransitionError{From: b.Status, To: BudgetApproved}
	}
	b.setStatus(BudgetApproved)
	return nil
}

// Cancel é a compensação do orçamento. Cancelar de novo não faz nada.
func (b *Budget) Cancel() {
	if b.Status != BudgetCancelled {
		b.setStatus(BudgetCancelled)
	}
}

func (b *Budget) setStatus(s BudgetStatus) {
	b.Status = s
	b.UpdatedAt = time.Now().UTC()
}

func (b *Budget) calculateTotals() {
	b.TotalServices, b.TotalParts = 0, 0
	for _, it := range b.Items {
		if it.Kind == ItemService {
			b.TotalServices += it.Subtotal()
		} else {
			b.TotalParts += it.Subtotal()
		}
	}
	b.TotalServices = roundMoney(b.TotalServices)
	b.TotalParts = roundMoney(b.TotalParts)
	b.Total = roundMoney(b.TotalServices + b.TotalParts)
}

func roundMoney(v float64) float64 {
	return math.Round(v*100) / 100
}

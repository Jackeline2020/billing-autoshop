package mocks

import (
	"context"
	"time"

	"billing-autoshop/internal/domain"
	"billing-autoshop/internal/gateway"

	"github.com/stretchr/testify/mock"
)

type PaymentRepositoryMock struct {
	mock.Mock
}

func (m *PaymentRepositoryMock) Create(p domain.Payment) error {
	return m.Called(p).Error(0)
}

func (m *PaymentRepositoryMock) FindByOrderID(orderID string) (domain.Payment, error) {
	args := m.Called(orderID)
	return args.Get(0).(domain.Payment), args.Error(1)
}

func (m *PaymentRepositoryMock) Update(p domain.Payment, previous domain.PaymentStatus) error {
	return m.Called(p, previous).Error(0)
}

func (m *PaymentRepositoryMock) FindExpired(now time.Time) ([]domain.Payment, error) {
	args := m.Called(now)
	return args.Get(0).([]domain.Payment), args.Error(1)
}

type GatewayMock struct {
	mock.Mock
}

func (m *GatewayMock) CreateCheckout(_ context.Context, req gateway.CheckoutRequest) (gateway.Checkout, error) {
	args := m.Called(req)
	return args.Get(0).(gateway.Checkout), args.Error(1)
}

func (m *GatewayMock) GetPayment(_ context.Context, id string) (gateway.ProviderPayment, error) {
	args := m.Called(id)
	return args.Get(0).(gateway.ProviderPayment), args.Error(1)
}

func (m *GatewayMock) Refund(_ context.Context, id string) error {
	return m.Called(id).Error(0)
}

// NotifierMock registra os avisos enviados ao orquestrador.
type NotifierMock struct {
	Confirmed []domain.Payment
	Failed    []domain.Payment
}

func (n *NotifierMock) PaymentConfirmed(p domain.Payment) { n.Confirmed = append(n.Confirmed, p) }
func (n *NotifierMock) PaymentFailed(p domain.Payment)    { n.Failed = append(n.Failed, p) }

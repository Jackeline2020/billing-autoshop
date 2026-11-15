package notifier_test

import (
	"errors"
	"testing"

	"billing-autoshop/internal/domain"
	"billing-autoshop/internal/messaging"
	"billing-autoshop/internal/mocks"
	"billing-autoshop/internal/notifier"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var payment = domain.Payment{OrderID: "os-1", SagaID: "saga-1", CorrelationID: "corr-1", Status: domain.PaymentRejected, FailureReason: "cc_rejected"}

func TestSagaNotifier_Confirmed(t *testing.T) {
	pub := new(mocks.PublisherMock)
	pub.On("Publish", messaging.EvtPaymentConfirmed, "os-1").Return(nil)

	notifier.NewSagaNotifier(pub).PaymentConfirmed(payment)
	require.Len(t, pub.Published, 1)
	assert.Equal(t, "saga-1", pub.Published[0].SagaID)
	assert.Equal(t, "corr-1", pub.Published[0].CorrelationID)
}

func TestSagaNotifier_Failed(t *testing.T) {
	pub := new(mocks.PublisherMock)
	pub.On("Publish", messaging.EvtPaymentFailed, "os-1").Return(nil)

	notifier.NewSagaNotifier(pub).PaymentFailed(payment)
	var p messaging.PaymentFailedPayload
	require.NoError(t, pub.Published[0].DecodePayload(&p))
	assert.Equal(t, "recusado", p.Status)
	assert.Equal(t, "cc_rejected", p.Reason)
}

func TestSagaNotifier_PublishError(t *testing.T) {
	pub := new(mocks.PublisherMock)
	pub.On("Publish", mock.Anything, mock.Anything).Return(errors.New("sqs fora"))
	notifier.NewSagaNotifier(pub).PaymentConfirmed(payment) // só registra no log
	assert.Len(t, pub.Published, 1)
}

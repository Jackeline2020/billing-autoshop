package messaging_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"billing-autoshop/internal/messaging"
	"billing-autoshop/internal/mocks"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEventAndDecode(t *testing.T) {
	env, err := messaging.NewEvent(messaging.EvtBudgetGenerated, "saga-1", "os-1", "corr-1",
		messaging.BudgetGeneratedPayload{BudgetID: "b1", Total: 80})
	require.NoError(t, err)
	assert.NotEmpty(t, env.MessageID)

	var p messaging.BudgetGeneratedPayload
	require.NoError(t, env.DecodePayload(&p))
	assert.Equal(t, 80.0, p.Total)
}

func TestEventWithoutPayload(t *testing.T) {
	env, err := messaging.NewEvent(messaging.EvtBudgetCancelled, "s", "o", "c", nil)
	require.NoError(t, err)
	assert.ErrorIs(t, env.DecodePayload(&struct{}{}), messaging.ErrEmptyPayload)

	_, err = messaging.NewEvent("x", "s", "o", "c", make(chan int))
	assert.Error(t, err)
}

func TestReplyToKeepsSagaContext(t *testing.T) {
	cmd := messaging.Envelope{MessageID: "m1", SagaID: "saga-1", OrderID: "os-1", CorrelationID: "corr-1"}
	evt, err := messaging.ReplyTo(cmd, messaging.EvtPaymentRefunded, nil)
	require.NoError(t, err)
	assert.Equal(t, "saga-1", evt.SagaID)
	assert.Equal(t, "corr-1", evt.CorrelationID)
	assert.NotEqual(t, "m1", evt.MessageID)
}

func TestPublisher(t *testing.T) {
	fake := &mocks.SQSFake{SendErrs: []error{errors.New("falha momentânea")}}
	pub := messaging.NewSQSPublisher(fake, "http://queue/os-events")
	env, _ := messaging.NewEvent(messaging.EvtPaymentConfirmed, "s", "o", "corr-1", nil)

	require.NoError(t, pub.Publish(context.Background(), env))
	require.Len(t, fake.Sent, 1)
	var sent messaging.Envelope
	require.NoError(t, json.Unmarshal([]byte(aws.ToString(fake.Sent[0].MessageBody)), &sent))
	assert.Equal(t, env.MessageID, sent.MessageID)
	assert.Equal(t, "corr-1", aws.ToString(fake.Sent[0].MessageAttributes["correlation_id"].StringValue))
}

func TestPublisher_GivesUp(t *testing.T) {
	boom := errors.New("fora do ar")
	fake := &mocks.SQSFake{SendErrs: []error{boom, boom, boom}}
	env, _ := messaging.NewEvent("x", "s", "o", "", nil)
	assert.ErrorIs(t, messaging.NewSQSPublisher(fake, "q").Publish(context.Background(), env), boom)
}

func TestQueueURL(t *testing.T) {
	fake := &mocks.SQSFake{QueueURLs: map[string]string{"billing-commands": "http://queue/billing-commands"}}
	url, err := messaging.QueueURL(context.Background(), fake, "billing-commands")
	require.NoError(t, err)
	assert.Equal(t, "http://queue/billing-commands", url)
	_, err = messaging.QueueURL(context.Background(), fake, "x")
	assert.Error(t, err)
}

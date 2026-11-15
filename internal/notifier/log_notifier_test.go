package notifier

import (
	"testing"

	"billing-autoshop/internal/domain"
)

func TestLogNotifier(t *testing.T) {
	n := LogNotifier{}
	n.PaymentConfirmed(domain.Payment{OrderID: "os-1"})
	n.PaymentFailed(domain.Payment{OrderID: "os-1", Status: domain.PaymentRejected})
}

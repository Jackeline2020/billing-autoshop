// Package worker contém rotinas que rodam em segundo plano no serviço.
package worker

import (
	"context"
	"time"

	"billing-autoshop/pkg/logger"
)

// Expirer encerra pagamentos pendentes cujo prazo passou.
type Expirer interface {
	ExpirePending(now time.Time) (int, error)
}

// RunExpiration verifica os pagamentos vencidos a cada intervalo, até o
// contexto ser cancelado.
func RunExpiration(ctx context.Context, e Expirer, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if _, err := e.ExpirePending(now); err != nil {
				logger.Error("erro ao expirar pagamentos", "error", err.Error())
			}
		}
	}
}

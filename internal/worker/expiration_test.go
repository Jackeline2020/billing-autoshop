package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type fakeExpirer struct {
	calls atomic.Int32
	err   error
}

func (f *fakeExpirer) ExpirePending(time.Time) (int, error) {
	f.calls.Add(1)
	return 0, f.err
}

func TestRunExpiration(t *testing.T) {
	for _, e := range []*fakeExpirer{{}, {err: errors.New("banco fora")}} {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { RunExpiration(ctx, e, 5*time.Millisecond); close(done) }()

		assert.Eventually(t, func() bool { return e.calls.Load() >= 2 }, time.Second, 5*time.Millisecond)
		cancel()
		<-done
	}
}

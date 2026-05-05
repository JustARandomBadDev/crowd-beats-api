package scheduler

import (
	"context"
	"time"

	"crowdbeats/internal/usecase"
)

type QueueRecalculator struct {
	interval time.Duration
	usecases *usecase.Services
	cancel   context.CancelFunc
}

func NewQueueRecalculator(interval time.Duration, usecases *usecase.Services) *QueueRecalculator {
	return &QueueRecalculator{
		interval: interval,
		usecases: usecases,
	}
}

func (q *QueueRecalculator) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	q.cancel = cancel
	go func() {
		ticker := time.NewTicker(q.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				q.usecases.RecalculateDirtyRooms(ctx)
			}
		}
	}()
}

func (q *QueueRecalculator) Stop() {
	if q.cancel != nil {
		q.cancel()
	}
}

package scheduler

import (
	"context"
	"log"
	"time"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/usecase"

	"github.com/google/uuid"
)

const pollInterval = time.Second

type QueueRecalculator struct {
	list        func(context.Context) ([]room.ScheduleEntry, error)
	recalculate func(context.Context, uuid.UUID) (bool, error)
	lastAttempt map[uuid.UUID]time.Time
	cancel      context.CancelFunc
	done        chan struct{}
}

func NewQueueRecalculator(usecases *usecase.Services) *QueueRecalculator {
	return &QueueRecalculator{
		list: usecases.ListQueueRooms, recalculate: usecases.RecalculateRoomQueue,
		lastAttempt: make(map[uuid.UUID]time.Time),
	}
}

func (q *QueueRecalculator) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	q.cancel = cancel
	q.done = make(chan struct{})
	go func() {
		defer close(q.done)
		// Every room is due on startup, even when no in-memory state survived.
		q.runDue(ctx, time.Now().UTC())
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				q.runDue(ctx, now)
			}
		}
	}()
}

func (q *QueueRecalculator) runDue(ctx context.Context, now time.Time) {
	entries, err := q.list(ctx)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("queue scheduler: list rooms: %v", err)
		}
		return
	}
	seen := make(map[uuid.UUID]struct{}, len(entries))
	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		seen[entry.ID] = struct{}{}
		interval := time.Duration(entry.RecalcIntervalSeconds) * time.Second
		if interval <= 0 {
			log.Printf("queue scheduler: invalid interval for room %s", entry.ID)
			interval = 10 * time.Second
		}
		if last, ok := q.lastAttempt[entry.ID]; ok && now.Sub(last) < interval {
			continue
		}
		q.lastAttempt[entry.ID] = now
		if _, err := q.recalculate(ctx, entry.ID); err != nil && ctx.Err() == nil {
			log.Printf("queue scheduler: recalculate room %s: %v", entry.ID, err)
		}
	}
	for roomID := range q.lastAttempt {
		if _, ok := seen[roomID]; !ok {
			delete(q.lastAttempt, roomID)
		}
	}
}

func (q *QueueRecalculator) Stop() {
	if q.cancel != nil {
		q.cancel()
		<-q.done
	}
}

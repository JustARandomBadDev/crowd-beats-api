package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"crowdbeats/internal/domain/room"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestQueueRecalculatorStartupAndPerRoomIntervals(t *testing.T) {
	fast, slow := uuid.New(), uuid.New()
	entries := []room.ScheduleEntry{
		{ID: fast, RecalcIntervalSeconds: 2},
		{ID: slow, RecalcIntervalSeconds: 10},
	}
	var calls []uuid.UUID
	q := &QueueRecalculator{
		list: func(context.Context) ([]room.ScheduleEntry, error) { return entries, nil },
		recalculate: func(_ context.Context, id uuid.UUID) (bool, error) {
			calls = append(calls, id)
			return true, nil
		},
		lastAttempt: make(map[uuid.UUID]time.Time),
	}
	start := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	q.runDue(context.Background(), start) // no MarkDirty or prior process state
	require.Equal(t, []uuid.UUID{fast, slow}, calls)
	q.runDue(context.Background(), start.Add(time.Second))
	require.Len(t, calls, 2)
	q.runDue(context.Background(), start.Add(2*time.Second))
	require.Equal(t, []uuid.UUID{fast, slow, fast}, calls)
	q.runDue(context.Background(), start.Add(10*time.Second))
	require.Equal(t, []uuid.UUID{fast, slow, fast, fast, slow}, calls)

	entries = entries[:1]
	q.runDue(context.Background(), start.Add(11*time.Second))
	_, tracked := q.lastAttempt[slow]
	require.False(t, tracked, "rooms no longer scheduled should not remain in memory")
}

func TestQueueRecalculatorRetriesFailedRoomAtNextInterval(t *testing.T) {
	id := uuid.New()
	callCount := 0
	q := &QueueRecalculator{
		list: func(context.Context) ([]room.ScheduleEntry, error) {
			return []room.ScheduleEntry{{ID: id, RecalcIntervalSeconds: 2}}, nil
		},
		recalculate: func(context.Context, uuid.UUID) (bool, error) {
			callCount++
			if callCount == 1 {
				return false, errors.New("temporary failure")
			}
			return true, nil
		},
		lastAttempt: make(map[uuid.UUID]time.Time),
	}
	start := time.Now()
	q.runDue(context.Background(), start)
	q.runDue(context.Background(), start.Add(time.Second))
	require.Equal(t, 1, callCount)
	q.runDue(context.Background(), start.Add(2*time.Second))
	require.Equal(t, 2, callCount)
}

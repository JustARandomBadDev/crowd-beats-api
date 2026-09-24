package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"crowdbeats/internal/domain/queue"
	"crowdbeats/internal/infra/http/dto"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestQueueSnapshotPublicShape(t *testing.T) {
	updatedAt := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	playingID := uuid.New()
	withoutPlaying, err := json.Marshal(dto.QueueSnapshotFromDomain(queue.Snapshot{UpdatedAt: &updatedAt}))
	require.NoError(t, err)
	require.JSONEq(t, `{"items":[],"now_playing":null,"updated_at":"2026-09-20T12:00:00Z"}`, string(withoutPlaying))
	withPlaying, err := json.Marshal(dto.QueueSnapshotFromDomain(queue.Snapshot{
		NowPlaying: &queue.Item{RoomTrackID: playingID, VoteCount: 2, SpotifyTrackID: "spotify-id", Title: "Playing", FIFOOrder: 99},
		UpdatedAt:  &updatedAt,
	}))
	require.NoError(t, err)
	var public struct {
		NowPlaying map[string]any `json:"now_playing"`
	}
	require.NoError(t, json.Unmarshal(withPlaying, &public))
	require.Equal(t, playingID.String(), public.NowPlaying["room_track_id"])
	require.NotContains(t, public.NowPlaying, "position")
	require.NotContains(t, public.NowPlaying, "fifo_order")
	require.NotContains(t, public.NowPlaying, "recalculated_at")
}

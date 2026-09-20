package usecase_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/testutil"
	"crowdbeats/internal/usecase"
	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func intPtr(v int) *int          { return &v }
func stringPtr(v string) *string { return &v }

func TestCreateRoomDefaultsAndValidation(t *testing.T) {
	h := testutil.NewHarness()
	var captured room.CreateInput
	h.Repos.RoomRepo.CreateFn = func(_ context.Context, input room.CreateInput) (room.Room, error) {
		captured = input
		return room.Room{ID: uuid.New(), Name: input.Name}, nil
	}
	_, _, err := h.Services.CreateRoom(context.Background(), usecase.CreateRoomInput{Name: "  Bar  "})
	require.NoError(t, err)
	require.Equal(t, "Bar", captured.Name)
	require.Equal(t, room.StatusDraft, captured.Status)
	require.Equal(t, 20, captured.QueueLimit)
	require.Equal(t, 5, captured.MaxVotesPerUser)
	require.Equal(t, 14400, captured.QRTTLSeconds)
	require.Equal(t, 10, captured.RecalcIntervalSeconds)
	require.Equal(t, "hashed:test-token", captured.ManagerSecretHash)

	for _, tc := range []struct {
		name  string
		input usecase.CreateRoomInput
	}{
		{"empty name", usecase.CreateRoomInput{Name: ""}},
		{"whitespace name", usecase.CreateRoomInput{Name: "   "}},
		{"zero recalc", usecase.CreateRoomInput{Name: "Bar", RecalcIntervalSeconds: intPtr(0)}},
		{"negative recalc", usecase.CreateRoomInput{Name: "Bar", RecalcIntervalSeconds: intPtr(-5)}},
		{"invalid status", usecase.CreateRoomInput{Name: "Bar", Status: stringPtr("unknown")}},
		{"invalid queue limit", usecase.CreateRoomInput{Name: "Bar", QueueLimit: intPtr(0)}},
		{"invalid vote limit", usecase.CreateRoomInput{Name: "Bar", MaxVotesPerUser: intPtr(21)}},
		{"invalid QR TTL", usecase.CreateRoomInput{Name: "Bar", QRTTLSeconds: intPtr(59)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := h.Services.CreateRoom(context.Background(), tc.input)
			var apiErr apierror.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, "VALIDATION_ERROR", apiErr.Code)
			require.Equal(t, http.StatusBadRequest, apiErr.Status)
		})
	}
}

func TestPatchRoomValidatesChangedFields(t *testing.T) {
	h := testutil.NewHarness()
	for _, patch := range []room.Patch{
		{Status: stringPtr("other")},
		{QueueLimit: intPtr(201)},
		{MaxVotesPerUser: intPtr(0)},
	} {
		_, err := h.Services.PatchRoom(context.Background(), uuid.New(), patch)
		var apiErr apierror.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, "VALIDATION_ERROR", apiErr.Code)
	}
}

func TestCreateQRCodeUsesRoomTTLAndValidatesOverride(t *testing.T) {
	h := testutil.NewHarness()
	roomID := uuid.New()
	locked := false
	h.Repos.RoomRepo.GetByIDForUpdateFn = func(_ context.Context, id uuid.UUID) (room.Room, error) {
		require.Equal(t, roomID, id)
		locked = true
		return room.Room{ID: roomID, QRTTLSeconds: 180}, nil
	}
	var expiresAt time.Time
	h.Repos.RoomRepo.RotateQRCodeFn = func(_ context.Context, id uuid.UUID, code string, expiry time.Time) error {
		require.True(t, locked)
		require.Equal(t, roomID, id)
		require.NotEmpty(t, code)
		expiresAt = expiry
		return nil
	}
	qr, err := h.Services.CreateQRCode(context.Background(), roomID, nil)
	require.NoError(t, err)
	require.Equal(t, h.Services.Now().Add(180*time.Second), expiresAt)
	require.Equal(t, expiresAt, qr.ExpiresAt)

	locked = false
	_, err = h.Services.CreateQRCode(context.Background(), roomID, intPtr(600))
	require.NoError(t, err)
	require.Equal(t, h.Services.Now().Add(600*time.Second), expiresAt)

	for _, invalid := range []int{0, -1, 59, 604801} {
		_, err := h.Services.CreateQRCode(context.Background(), roomID, &invalid)
		var apiErr apierror.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, "VALIDATION_ERROR", apiErr.Code)
	}
}

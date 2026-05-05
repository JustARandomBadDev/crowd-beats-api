package usecase_test

import (
	"context"
	"net/http"
	"testing"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/testutil"
	"crowdbeats/pkg/apierror"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestJoinByQRCodeRejectsInvalidQRCode(t *testing.T) {
	h := testutil.NewHarness()
	h.Repos.RoomRepo.GetByQRCodeFn = func(context.Context, string) (room.Room, error) {
		return room.Room{}, pgx.ErrNoRows
	}

	_, err := h.Services.JoinByQRCode(context.Background(), "invalid", "alex", "")
	require.Error(t, err)

	var apiErr apierror.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "INVALID_QR_CODE", apiErr.Code)
	require.Equal(t, http.StatusForbidden, apiErr.Status)
}

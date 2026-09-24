package dto

import (
	"time"

	"crowdbeats/internal/domain/queue"
	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/domain/spotify"
	"crowdbeats/internal/usecase"

	"github.com/google/uuid"
)

type RoomResponse struct {
	ID                    uuid.UUID `json:"id"`
	Name                  string    `json:"name"`
	Slug                  *string   `json:"slug"`
	Status                string    `json:"status"`
	QueueLimit            int       `json:"queue_limit"`
	MaxVotesPerUser       int       `json:"max_votes_per_user"`
	QRTTLSeconds          int       `json:"qr_ttl_seconds"`
	RecalcIntervalSeconds int       `json:"recalc_interval_seconds"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func RoomFromDomain(value room.Room) RoomResponse {
	return RoomResponse{
		ID: value.ID, Name: value.Name, Slug: value.Slug, Status: value.Status,
		QueueLimit: value.QueueLimit, MaxVotesPerUser: value.MaxVotesPerUser,
		QRTTLSeconds: value.QRTTLSeconds, RecalcIntervalSeconds: value.RecalcIntervalSeconds,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

type SessionResponse struct {
	ID         uuid.UUID `json:"id"`
	RoomID     uuid.UUID `json:"room_id"`
	Nickname   string    `json:"nickname"`
	Role       string    `json:"role"`
	Status     string    `json:"status"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

func SessionFromDomain(value session.Session) SessionResponse {
	return SessionResponse{
		ID: value.ID, RoomID: value.RoomID, Nickname: value.Nickname,
		Role: value.Role, Status: value.Status, LastSeenAt: value.LastSeenAt,
	}
}

type JoinSessionResponse struct {
	ID       uuid.UUID `json:"id"`
	Nickname string    `json:"nickname"`
	Role     string    `json:"role"`
	Token    string    `json:"token"`
}

type WebSocketResponse struct {
	URL string `json:"url"`
}

type JoinRoomResponse struct {
	Room    RoomResponse        `json:"room"`
	Session JoinSessionResponse `json:"session"`
	WS      WebSocketResponse   `json:"ws"`
}

type CreateRoomResponse struct {
	Room          RoomResponse `json:"room"`
	ManagerSecret string       `json:"manager_secret"`
}

type RoomContainerResponse struct {
	Room RoomResponse `json:"room"`
}

type SessionContainerResponse struct {
	Session SessionResponse `json:"session"`
}

type QRCodeResponse struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

func QRCodeFromDomain(value room.QRCode) QRCodeResponse {
	return QRCodeResponse{Code: value.Code, ExpiresAt: value.ExpiresAt}
}

type TopTrackResponse struct {
	Title       string `json:"title"`
	ArtistNames string `json:"artist_names"`
	Votes       int    `json:"votes"`
}

type RoomStatsResponse struct {
	ActiveUsers   int                `json:"active_users"`
	TracksInQueue int                `json:"tracks_in_queue"`
	VotesCount    int                `json:"votes_count"`
	TopTracks     []TopTrackResponse `json:"top_tracks"`
}

func RoomStatsFromDomain(value room.Stats) RoomStatsResponse {
	result := RoomStatsResponse{
		ActiveUsers: value.ActiveUsers, TracksInQueue: value.TracksInQueue,
		VotesCount: value.VotesCount, TopTracks: make([]TopTrackResponse, 0, len(value.TopTracks)),
	}
	for _, item := range value.TopTracks {
		result.TopTracks = append(result.TopTracks, TopTrackResponse{
			Title: item.Title, ArtistNames: item.ArtistNames, Votes: item.Votes,
		})
	}
	return result
}

type SpotifyTrackResponse struct {
	SpotifyTrackID string `json:"spotify_track_id"`
	Title          string `json:"title"`
	ArtistNames    string `json:"artist_names"`
	AlbumName      string `json:"album_name"`
	DurationMS     int    `json:"duration_ms"`
	ImageURL       string `json:"image_url"`
	PreviewURL     string `json:"preview_url"`
	URI            string `json:"uri"`
}

func SpotifyTrackFromDomain(value spotify.Track) SpotifyTrackResponse {
	return SpotifyTrackResponse{
		SpotifyTrackID: value.SpotifyTrackID, Title: value.Title,
		ArtistNames: value.ArtistNames, AlbumName: value.AlbumName,
		DurationMS: value.DurationMS, ImageURL: value.ImageURL,
		PreviewURL: value.PreviewURL, URI: value.URI,
	}
}

type SpotifySearchResponse struct {
	Items []SpotifyTrackResponse `json:"items"`
}

type QueueItemResponse struct {
	Position    int                  `json:"position"`
	RoomTrackID uuid.UUID            `json:"room_track_id"`
	Score       int                  `json:"score"`
	VoteCount   int                  `json:"vote_count"`
	Track       SpotifyTrackResponse `json:"track"`
	ProposedBy  *string              `json:"proposed_by"`
}

func QueueItemsFromDomain(items []queue.Item) []QueueItemResponse {
	result := make([]QueueItemResponse, 0, len(items))
	for _, item := range items {
		result = append(result, QueueItemResponse{
			Position: item.Position, RoomTrackID: item.RoomTrackID,
			Score: item.Score, VoteCount: item.VoteCount, ProposedBy: item.ProposedBy,
			Track: spotifyTrackFromQueueItem(item),
		})
	}
	return result
}

type NowPlayingResponse struct {
	RoomTrackID uuid.UUID            `json:"room_track_id"`
	VoteCount   int                  `json:"vote_count"`
	Track       SpotifyTrackResponse `json:"track"`
	ProposedBy  *string              `json:"proposed_by"`
}

func spotifyTrackFromQueueItem(item queue.Item) SpotifyTrackResponse {
	return SpotifyTrackResponse{
		SpotifyTrackID: item.SpotifyTrackID, Title: item.Title,
		ArtistNames: item.ArtistNames, AlbumName: item.AlbumName,
		DurationMS: item.DurationMS, ImageURL: item.ImageURL,
		PreviewURL: item.PreviewURL, URI: item.URI,
	}
}

type QueueSnapshotResponse struct {
	Items      []QueueItemResponse `json:"items"`
	NowPlaying *NowPlayingResponse `json:"now_playing"`
	UpdatedAt  *time.Time          `json:"updated_at"`
}

func QueueSnapshotFromDomain(snapshot queue.Snapshot) QueueSnapshotResponse {
	result := QueueSnapshotResponse{Items: QueueItemsFromDomain(snapshot.Items), UpdatedAt: snapshot.UpdatedAt}
	if snapshot.NowPlaying != nil {
		result.NowPlaying = &NowPlayingResponse{
			RoomTrackID: snapshot.NowPlaying.RoomTrackID,
			VoteCount:   snapshot.NowPlaying.VoteCount,
			Track:       spotifyTrackFromQueueItem(*snapshot.NowPlaying),
			ProposedBy:  snapshot.NowPlaying.ProposedBy,
		}
	}
	return result
}

type ProposedRoomTrackResponse struct {
	ID       uuid.UUID `json:"id"`
	Status   string    `json:"status"`
	Position *int      `json:"position"`
}

type ExistingRoomTrackResponse struct {
	ID               uuid.UUID `json:"id"`
	CurrentVoteCount int       `json:"current_vote_count"`
	Position         *int      `json:"position"`
}

type ProposeTrackResponse struct {
	RoomTrack         *ProposedRoomTrackResponse `json:"room_track,omitempty"`
	ExistingRoomTrack *ExistingRoomTrackResponse `json:"existing_room_track,omitempty"`
	Duplicate         bool                       `json:"duplicate"`
}

func ProposeTrackFromResult(value usecase.ProposeTrackResult) ProposeTrackResponse {
	result := ProposeTrackResponse{Duplicate: value.Duplicate}
	if value.RoomTrack != nil {
		result.RoomTrack = &ProposedRoomTrackResponse{
			ID: value.RoomTrack.ID, Status: value.RoomTrack.Status, Position: value.RoomTrack.Position,
		}
	}
	if value.ExistingRoomTrack != nil {
		result.ExistingRoomTrack = &ExistingRoomTrackResponse{
			ID:               value.ExistingRoomTrack.ID,
			CurrentVoteCount: value.ExistingRoomTrack.CurrentVoteCount,
			Position:         value.ExistingRoomTrack.Position,
		}
	}
	return result
}

type VoteResponse struct {
	VoteAdded        bool      `json:"vote_added"`
	RoomTrackID      uuid.UUID `json:"room_track_id"`
	CurrentVoteCount int       `json:"current_vote_count"`
	VotesRemaining   int       `json:"votes_remaining"`
}

func VoteFromResult(value usecase.AddVoteResult) VoteResponse {
	return VoteResponse{
		VoteAdded: value.VoteAdded, RoomTrackID: value.RoomTrackID,
		CurrentVoteCount: value.CurrentVoteCount, VotesRemaining: value.VotesRemaining,
	}
}

type TrackActionResponse struct {
	Updated     bool      `json:"updated"`
	RoomTrackID uuid.UUID `json:"room_track_id"`
	Status      string    `json:"status"`
}

type DeleteTrackResponse struct {
	Deleted     bool      `json:"deleted"`
	RoomTrackID uuid.UUID `json:"room_track_id"`
}

package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// SynapseAdmin provides methods for Synapse Admin API operations.
// All methods use the appservice token which must belong to a server admin user.
type SynapseAdmin struct {
	client *mautrix.Client
}

// NewSynapseAdmin creates a new SynapseAdmin client.
func NewSynapseAdmin(homeserverURL, accessToken string) (*SynapseAdmin, error) {
	hsURL, err := url.Parse(homeserverURL)
	if err != nil {
		return nil, fmt.Errorf("invalid homeserver URL %q: %w", homeserverURL, err)
	}
	return &SynapseAdmin{
		client: &mautrix.Client{
			HomeserverURL: hsURL,
			AccessToken:   accessToken,
			Client:        http.DefaultClient,
		},
	}, nil
}

func (s *SynapseAdmin) buildURL(path ...any) string {
	return s.client.BuildURL(mautrix.SynapseAdminURLPath(path))
}

// ============================================================================
// User Operations
// ============================================================================

// UserInfo contains user details from the admin API.
type UserInfo struct {
	Admin bool `json:"admin"`
}

// GetUser retrieves user details.
func (s *SynapseAdmin) GetUser(ctx context.Context, userID id.UserID) (*UserInfo, error) {
	var resp UserInfo
	_, err := s.client.MakeRequest(ctx, http.MethodGet, s.buildURL("v2", "users", userID), nil, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// SetUserAdmin sets or removes server admin status for a user.
func (s *SynapseAdmin) SetUserAdmin(ctx context.Context, userID id.UserID, admin bool) error {
	_, err := s.client.MakeRequest(ctx, http.MethodPut, s.buildURL("v2", "users", userID), map[string]interface{}{
		"admin": admin,
	}, nil)
	return err
}

// DeactivateUser deactivates and optionally erases a user.
func (s *SynapseAdmin) DeactivateUser(ctx context.Context, userID id.UserID, erase bool) error {
	_, err := s.client.MakeRequest(ctx, http.MethodPost, s.buildURL("v1", "deactivate", userID), map[string]interface{}{
		"erase": erase,
	}, nil)
	return err
}

// ============================================================================
// Registration (Shared Secret)
// ============================================================================

// RegisterResponse contains the registration result.
type RegisterResponse struct {
	AccessToken string `json:"access_token"`
}

// GetRegistrationNonce fetches a nonce for shared secret registration.
func (s *SynapseAdmin) GetRegistrationNonce(ctx context.Context) (string, error) {
	var resp struct {
		Nonce string `json:"nonce"`
	}
	_, err := s.client.MakeRequest(ctx, http.MethodGet, s.buildURL("v1", "register"), nil, &resp)
	if err != nil {
		return "", err
	}
	return resp.Nonce, nil
}

// RegisterWithMAC registers a user using a pre-computed HMAC.
func (s *SynapseAdmin) RegisterWithMAC(ctx context.Context, nonce, username, password, mac string, admin bool) (string, error) {
	var resp RegisterResponse
	_, err := s.client.MakeRequest(ctx, http.MethodPost, s.buildURL("v1", "register"), map[string]interface{}{
		"nonce":    nonce,
		"username": username,
		"password": password,
		"admin":    admin,
		"mac":      mac,
	}, &resp)
	if err != nil {
		return "", err
	}
	return resp.AccessToken, nil
}

// ============================================================================
// Room Operations
// ============================================================================

// AdminRoom contains room details from the admin API.
type AdminRoom struct {
	RoomID         id.RoomID `json:"room_id"`
	RoomType       string    `json:"room_type"`
	CanonicalAlias string    `json:"canonical_alias"`
}

// ListRooms returns all rooms on the server.
func (s *SynapseAdmin) ListRooms(ctx context.Context, limit int) ([]AdminRoom, error) {
	var resp struct {
		Rooms []AdminRoom `json:"rooms"`
	}
	urlPath := fmt.Sprintf("%s?limit=%d", s.buildURL("v1", "rooms"), limit)
	_, err := s.client.MakeRequest(ctx, http.MethodGet, urlPath, nil, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Rooms, nil
}

// GetRoomMembers returns the member list for a room as strings.
func (s *SynapseAdmin) GetRoomMembers(ctx context.Context, roomID id.RoomID) ([]string, error) {
	var resp struct {
		Members []string `json:"members"`
	}
	_, err := s.client.MakeRequest(ctx, http.MethodGet, s.buildURL("v1", "rooms", roomID, "members"), nil, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Members, nil
}

// GetRoomMemberIDs returns the member list for a room as typed UserIDs.
func (s *SynapseAdmin) GetRoomMemberIDs(ctx context.Context, roomID id.RoomID) ([]id.UserID, error) {
	members, err := s.GetRoomMembers(ctx, roomID)
	if err != nil {
		return nil, err
	}
	result := make([]id.UserID, len(members))
	for i, m := range members {
		result[i] = id.UserID(m)
	}
	return result, nil
}

// GetRoomState retrieves room state events, optionally filtered by type.
// If eventType is empty, returns all state events.
func (s *SynapseAdmin) GetRoomState(ctx context.Context, roomID id.RoomID, eventType string) ([]json.RawMessage, error) {
	var resp struct {
		State []json.RawMessage `json:"state"`
	}
	urlPath := s.buildURL("v1", "rooms", roomID, "state")
	if eventType != "" {
		urlPath += "?type=" + url.QueryEscape(eventType)
	}
	_, err := s.client.MakeRequest(ctx, http.MethodGet, urlPath, nil, &resp)
	if err != nil {
		return nil, err
	}
	return resp.State, nil
}

// GetStateEventContent retrieves the content of a specific state event by type.
// Returns nil if the event doesn't exist.
func (s *SynapseAdmin) GetStateEventContent(ctx context.Context, roomID id.RoomID, eventType string) (map[string]interface{}, error) {
	stateEvents, err := s.GetRoomState(ctx, roomID, eventType)
	if err != nil || len(stateEvents) == 0 {
		return nil, err
	}
	var evt struct {
		Content map[string]interface{} `json:"content"`
	}
	if err := json.Unmarshal(stateEvents[0], &evt); err != nil {
		return nil, err
	}
	return evt.Content, nil
}

// GetCustomState retrieves io.alkemio.* state events from a room.
// If eventTypes is empty, returns all io.alkemio.* state events.
func (s *SynapseAdmin) GetCustomState(ctx context.Context, roomID id.RoomID, eventTypes []string) (map[string]map[string]interface{}, error) {
	result := make(map[string]map[string]interface{})

	if len(eventTypes) > 0 {
		// Fetch specific types
		for _, et := range eventTypes {
			if !strings.HasPrefix(et, "io.alkemio.") {
				return nil, fmt.Errorf("custom state event type must have io.alkemio. prefix, got: %s", et)
			}
			stateEvents, err := s.GetRoomState(ctx, roomID, et)
			if err != nil {
				return nil, fmt.Errorf("failed to get state for %s in %s: %w", et, roomID, err)
			}
			if len(stateEvents) == 0 {
				continue
			}
			var evt struct {
				Content map[string]interface{} `json:"content"`
			}
			if err := json.Unmarshal(stateEvents[0], &evt); err != nil {
				return nil, fmt.Errorf("failed to unmarshal state event %s: %w", et, err)
			}
			if len(evt.Content) > 0 {
				result[et] = evt.Content
			}
		}
	} else {
		// Fetch all state and filter
		stateEvents, err := s.GetRoomState(ctx, roomID, "")
		if err != nil {
			return nil, err
		}
		for _, raw := range stateEvents {
			var evt struct {
				Type    string                 `json:"type"`
				Content map[string]interface{} `json:"content"`
			}
			if err := json.Unmarshal(raw, &evt); err != nil {
				continue // skip non-JSON state events (e.g. binary or malformed)
			}
			if strings.HasPrefix(evt.Type, "io.alkemio.") && len(evt.Content) > 0 {
				result[evt.Type] = evt.Content
			}
		}
	}

	return result, nil
}

// ============================================================================
// Room Content Operations (messages, events)
// ============================================================================

// GetRoomMessages retrieves messages from a room. Mirrors the client API /messages endpoint.
// dir: "b" for backwards (newest first), "f" for forwards (oldest first).
func (s *SynapseAdmin) GetRoomMessages(ctx context.Context, roomID id.RoomID, from, dir string, limit int) (*mautrix.RespMessages, error) {
	query := url.Values{}
	if from != "" {
		query.Set("from", from)
	}
	query.Set("dir", dir)
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	urlPath := fmt.Sprintf("%s?%s", s.buildURL("v1", "rooms", roomID, "messages"), query.Encode())

	var resp mautrix.RespMessages
	_, err := s.client.MakeRequest(ctx, http.MethodGet, urlPath, nil, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetEventContext retrieves a single event with context from a room.
func (s *SynapseAdmin) GetEventContext(ctx context.Context, roomID id.RoomID, eventID id.EventID) (*mautrix.RespContext, error) {
	var resp mautrix.RespContext
	urlPath := s.buildURL("v1", "rooms", roomID, "context", eventID)
	_, err := s.client.MakeRequest(ctx, http.MethodGet, urlPath+"?limit=0", nil, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetEvent retrieves a single event from a room (convenience wrapper around GetEventContext).
func (s *SynapseAdmin) GetEvent(ctx context.Context, roomID id.RoomID, eventID id.EventID) (*event.Event, error) {
	resp, err := s.GetEventContext(ctx, roomID, eventID)
	if err != nil {
		return nil, err
	}
	return resp.Event, nil
}

// GetTimestampToEvent finds the event closest to the given timestamp.
func (s *SynapseAdmin) GetTimestampToEvent(ctx context.Context, roomID id.RoomID, ts int64, dir string) (id.EventID, error) {
	query := url.Values{}
	query.Set("ts", fmt.Sprintf("%d", ts))
	query.Set("dir", dir)
	urlPath := fmt.Sprintf("%s?%s", s.buildURL("v1", "rooms", roomID, "timestamp_to_event"), query.Encode())

	var resp struct {
		EventID id.EventID `json:"event_id"`
	}
	_, err := s.client.MakeRequest(ctx, http.MethodGet, urlPath, nil, &resp)
	if err != nil {
		return "", err
	}
	return resp.EventID, nil
}

// maxRelationsPages bounds GetRelations pagination so a pathological thread can
// never loop unbounded. This is an HONEST cap: a thread with more than
// maxRelationsPages*relationsPageLimit newer relations before an older reply will
// NOT return that older reply — an accepted bound to keep admin round-trips
// finite.
const maxRelationsPages = 20

// relationsFetchBudget bounds the WHOLE GetRelations pagination loop.
//
// Pagination turned one admin round-trip into up to maxRelationsPages sequential
// ones, and each runs on SynapseAdmin's http.DefaultClient (no Client.Timeout)
// with whatever context the caller had. The callers are the watermill room-ops
// handlers, whose message context carries NO deadline and which process messages
// SEQUENTIALLY in one goroutine — so an unresponsive homeserver would otherwise
// stall every room operation behind it, for as long as the TCP stack allows,
// multiplied by the page count. One budget for the whole loop keeps that finite
// regardless of how many pages are walked, and it is a CEILING: a caller with a
// shorter deadline still wins.
//
// Exhausting it mid-pagination is not a special case: the request context
// expires, MakeRequest fails, and the (partial, err) contract below already
// covers "a later page failed", so callers degrade exactly as they do for a
// transport error.
const relationsFetchBudget = 60 * time.Second

// relationsPageLimit is the `limit` GetRelations asks for on every page.
//
// It MUST be explicit. The Matrix `/relations` endpoint's default page size is
// the SERVER's choice, and Synapse's is 5 — so an unspecified limit would make
// the real bound ~100 relations across 20 pages, not the thousands the page cap
// suggests, and would silently drop older thread replies on any busy thread. 100
// is the largest value servers are expected to honour (they may clamp lower,
// which only costs extra round-trips within the same page cap).
const relationsPageLimit = 100

// ErrRelationsTruncated reports that GetRelations stopped at maxRelationsPages
// while the homeserver still had more pages. It is returned ALONGSIDE the pages
// already fetched, using the same (partial, err) contract as a transport
// failure, so truncation is never silent: thread reads log it and serve the
// partial set, reaction lookups propagate it instead of reporting "not found".
var ErrRelationsTruncated = errors.New("relations pagination hit the page cap; results are truncated")

// GetRelations retrieves relations (reactions, threads) for an event.
// Uses the Matrix Client API (not Synapse Admin API) since no admin relations endpoint exists.
// The admin access token is a normal client token — standard room access rules apply.
//
// An empty eventType (event.Type{}) omits the type path segment, so the endpoint
// returns ALL event types carrying the relation — required to include m.sticker
// thread replies alongside m.room.message replies (the caller filters
// client-side via isMessageLikeEvent). A non-empty eventType filters server-side
// (e.g. m.reaction for annotation lookups).
//
// The fetch is PAGINATED: it asks for relationsPageLimit per page, follows the
// response next_batch token (bounded by maxRelationsPages) and accumulates every
// page. A single unpaginated page of newest-first relations could otherwise push
// older replies off the end — e.g. many recent sticker replies burying an older
// message reply — silently dropping real replies.
//
// On error it returns BOTH the pages accumulated so far AND the error, leaving the
// best-effort-vs-fail decision to each caller (this is a SHARED helper — thread
// reads want partial replies, reaction lookups must not treat an error as
// "not found"):
//   - the FIRST page failing → return (nil, err): nothing accumulated yet.
//   - a LATER page failing (including a context deadline mid-pagination) → return
//     (partial, err): the pages fetched so far plus the error, so a caller can
//     use the partial set or propagate as it sees fit.
//   - the page cap being reached with more pages still available → return
//     (partial, ErrRelationsTruncated), so truncation travels the SAME visible
//     path as a transport failure instead of being silently indistinguishable
//     from a complete result. SynapseAdmin has no logger of its own; its callers
//     already log the (partial, err) case.
func (s *SynapseAdmin) GetRelations(
	ctx context.Context, roomID id.RoomID, eventID id.EventID,
	relType event.RelationType, eventType event.Type,
) ([]*event.Event, error) {
	// One deadline for the whole page walk — see relationsFetchBudget.
	ctx, cancel := context.WithTimeout(ctx, relationsFetchBudget)
	defer cancel()

	var base string
	if eventType.Type == "" {
		base = s.client.BuildClientURL("v1", "rooms", roomID, "relations", eventID, relType)
	} else {
		base = s.client.BuildClientURL("v1", "rooms", roomID, "relations", eventID, relType, eventType.Type)
	}
	// The limit is a query parameter on EVERY page (the from= token below is
	// appended to this), never left to the server's default.
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	base += sep + "limit=" + strconv.Itoa(relationsPageLimit)

	var all []*event.Event
	from := ""
	for page := 0; page < maxRelationsPages; page++ {
		urlPath := base
		if from != "" {
			urlPath = base + "&from=" + url.QueryEscape(from)
		}

		var resp struct {
			Chunk     []*event.Event `json:"chunk"`
			NextBatch string         `json:"next_batch"`
		}
		if _, err := s.client.MakeRequest(ctx, http.MethodGet, urlPath, nil, &resp); err != nil {
			if page == 0 {
				return nil, err
			}
			// Later-page failure (transient error or context deadline): return what
			// we already fetched ALONGSIDE the error, so the caller decides whether to
			// use the partial set (thread reads) or propagate (reaction lookups).
			return all, err
		}
		all = append(all, resp.Chunk...)
		if resp.NextBatch == "" {
			return all, nil
		}
		from = resp.NextBatch
	}
	// Loop exhausted with next_batch still set: more relations exist than the page
	// cap allows. Surface it rather than returning a truncated set as complete.
	return all, ErrRelationsTruncated
}

// JoinRoom forces a user to join a room (bypasses join rules for admin).
func (s *SynapseAdmin) JoinRoom(ctx context.Context, roomID id.RoomID, userID id.UserID) error {
	_, err := s.client.MakeRequest(ctx, http.MethodPost, s.buildURL("v1", "join", roomID), map[string]interface{}{
		"user_id": userID,
	}, nil)
	return err
}

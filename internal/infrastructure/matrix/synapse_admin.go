package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// SynapseAdmin provides methods for Synapse Admin API operations.
// All methods use the appservice token which must belong to a server admin user.
type SynapseAdmin struct {
	client *mautrix.Client
}

// NewSynapseAdmin creates a new SynapseAdmin client.
func NewSynapseAdmin(homeserverURL, accessToken string) *SynapseAdmin {
	hsURL, _ := url.Parse(homeserverURL)
	return &SynapseAdmin{
		client: &mautrix.Client{
			HomeserverURL: hsURL,
			AccessToken:   accessToken,
			Client:        http.DefaultClient,
		},
	}
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

// GetUser retrieves user details. Returns nil if user doesn't exist.
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

// GetRoomMembers returns the member list for a room.
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
				continue
			}
			stateEvents, err := s.GetRoomState(ctx, roomID, et)
			if err != nil || len(stateEvents) == 0 {
				continue
			}
			var evt struct {
				Content map[string]interface{} `json:"content"`
			}
			if err := json.Unmarshal(stateEvents[0], &evt); err == nil && len(evt.Content) > 0 {
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
			if err := json.Unmarshal(raw, &evt); err == nil {
				if strings.HasPrefix(evt.Type, "io.alkemio.") && len(evt.Content) > 0 {
					result[evt.Type] = evt.Content
				}
			}
		}
	}

	return result, nil
}

package matrix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/config"
	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/testutil"
)

// testASToken is a sentinel, deliberately NOT empty: mautrix sets the
// Authorization header only when the token is non-empty, so an empty token
// would exercise an UNAUTHENTICATED login and still satisfy every other case
// below. Case 6 asserts the header, which is the mechanism's only
// authentication.
const testASToken = "as-token-sentinel-091"

// testAccessTokenSentinel is the minted credential; no log entry may contain it.
const testAccessTokenSentinel = "syt_SENTINEL_minted_access_token" //nolint:gosec // test sentinel

// newSessionTestAdapter points the package's standard test adapter at hsURL.
// No other helper sets Matrix.HomeserverURL, which newDirectClient dereferences.
func newSessionTestAdapter(
	hsURL string, intent *mockIntentAPI, log *testutil.RecordingLogger,
) *MautrixAdapter {
	as := newMockAS(intent, map[id.UserID]intentAPI{expectedUserID(testActorID): intent})
	a := newFullTestAdapter(as, &mockAdminAPI{})
	a.logger = log
	a.cfg = &config.Config{}
	a.cfg.Matrix.HomeserverURL = hsURL
	a.cfg.Matrix.AppServiceToken = testASToken
	return a
}

// loginCapture records what the homeserver received for the one login call.
type loginCapture struct {
	body                   map[string]interface{}
	authorization          string
	ensureRegisteredBefore int
}

// serveLogin starts a homeserver stub that answers /login with respBody and
// records the request. respBody is written as-is so a test can omit fields.
func serveLogin(
	t *testing.T, intent *mockIntentAPI, respBody map[string]interface{},
) (*MautrixAdapter, *loginCapture, *testutil.RecordingLogger) {
	t.Helper()

	capture := &loginCapture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/_matrix/client/v3/login", r.URL.Path)
		capture.authorization = r.Header.Get("Authorization")
		capture.ensureRegisteredBefore = intent.ensureRegisteredCalled
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capture.body))
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(respBody))
	}))
	t.Cleanup(server.Close)

	log := &testutil.RecordingLogger{}
	return newSessionTestAdapter(server.URL, intent, log), capture, log
}

// completeLoginResponse is the shape HG-0 observed from Synapse v1.132.0.
func completeLoginResponse() map[string]interface{} {
	return map[string]interface{}{
		"user_id":       string(expectedUserID(testActorID)),
		"device_id":     "BOEPBOALLR",
		"access_token":  testAccessTokenSentinel,
		"expires_in_ms": 172800000,
	}
}

func TestCreateUserSession_LoginRequestShape(t *testing.T) {
	intent := &mockIntentAPI{}
	adapter, capture, _ := serveLogin(t, intent, completeLoginResponse())

	session, err := adapter.CreateUserSession(context.Background(), domain.Actor{ID: testActorID})
	require.NoError(t, err)
	assert.Equal(t, testAccessTokenSentinel, session.AccessToken)

	assert.Equal(t, "m.login.application_service", capture.body["type"])
	identifier, ok := capture.body["identifier"].(map[string]interface{})
	require.True(t, ok, "identifier must be an object")
	assert.Equal(t, "m.id.user", identifier["type"])
	assert.Equal(t, "@"+testActorID.String()+":test.local", identifier["user"])
	assert.Equal(t, "Alkemio App", capture.body["initial_device_display_name"])

	// A stable device would appear as a present key (ReqLogin.DeviceID is
	// omitempty), so absence is the assertion that reds for it.
	assert.NotContains(t, capture.body, "device_id")
	assert.NotContains(t, capture.body, "refresh_token")
}

func TestCreateUserSession_LogsExpiryButNeverTheToken(t *testing.T) {
	intent := &mockIntentAPI{}
	adapter, _, log := serveLogin(t, intent, completeLoginResponse())

	_, err := adapter.CreateUserSession(context.Background(), domain.Actor{ID: testActorID})
	require.NoError(t, err)

	var expiryLogged bool
	for _, entry := range log.Entries {
		assert.NotContains(t, entry, testAccessTokenSentinel, "access token leaked into a log entry")
		if strings.Contains(entry, "expires_in_ms") && strings.Contains(entry, "172800000") {
			expiryLogged = true
		}
	}
	assert.True(t, expiryLogged, "expected expires_in_ms and its value in a log entry, got %v", log.Entries)
}

func TestCreateUserSession_RegistersGhostBeforeLogin(t *testing.T) {
	intent := &mockIntentAPI{}
	adapter, capture, _ := serveLogin(t, intent, completeLoginResponse())

	_, err := adapter.CreateUserSession(context.Background(), domain.Actor{ID: testActorID})
	require.NoError(t, err)

	assert.Equal(t, 1, intent.ensureRegisteredCalled)
	assert.Equal(t, 1, capture.ensureRegisteredBefore, "the ghost must be registered before the login is sent")
}

func TestCreateUserSession_RejectsDivergentMXID(t *testing.T) {
	mapped := expectedUserID(testActorID)

	tests := map[string]string{
		"different localpart":  "@" + testActorID2.String() + ":test.local",
		"different homeserver": string(mapped[:strings.Index(string(mapped), ":")]) + ":evil.local",
	}
	for name, returnedMXID := range tests {
		t.Run(name, func(t *testing.T) {
			resp := completeLoginResponse()
			resp["user_id"] = returnedMXID
			adapter, _, _ := serveLogin(t, &mockIntentAPI{}, resp)

			session, err := adapter.CreateUserSession(context.Background(), domain.Actor{ID: testActorID})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "minted session identity mismatch")
			assert.Nil(t, session)
		})
	}

	t.Run("requested MXID is accepted", func(t *testing.T) {
		adapter, _, _ := serveLogin(t, &mockIntentAPI{}, completeLoginResponse())

		session, err := adapter.CreateUserSession(context.Background(), domain.Actor{ID: testActorID})
		require.NoError(t, err)
		assert.Equal(t, mapped, session.UserID)
		assert.Equal(t, id.DeviceID("BOEPBOALLR"), session.DeviceID)
	})
}

func TestCreateUserSession_RejectsIncompleteResponse(t *testing.T) {
	for _, missing := range []string{"user_id", "device_id", "access_token"} {
		t.Run("missing "+missing, func(t *testing.T) {
			resp := completeLoginResponse()
			resp[missing] = ""
			adapter, _, _ := serveLogin(t, &mockIntentAPI{}, resp)

			session, err := adapter.CreateUserSession(context.Background(), domain.Actor{ID: testActorID})
			require.Error(t, err)
			assert.Nil(t, session)
		})
	}
}

func TestCreateUserSession_LoginUsesAppServiceToken(t *testing.T) {
	adapter, capture, _ := serveLogin(t, &mockIntentAPI{}, completeLoginResponse())

	_, err := adapter.CreateUserSession(context.Background(), domain.Actor{ID: testActorID})
	require.NoError(t, err)

	assert.Equal(t, "Bearer "+testASToken, capture.authorization)
}

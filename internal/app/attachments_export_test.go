package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alkem-io/matrix-adapter/internal/config"
)

const exportRoom = "11111111-1111-4111-8111-111111111111"
const exportOtherRoom = "22222222-2222-4222-8222-222222222222"
const exportActor = "33333333-3333-4333-8333-333333333333"
const exportDocument = "44444444-4444-4444-8444-444444444444"

type exportHTTPFixture struct {
	aliases        []string
	resolveError   bool
	unresolvedRoom string
	rooms          []string
	pages          map[string]string
	requestedFrom  []string
}

func (f *exportHTTPFixture) serve(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "export must never reconcile, register, join or write state")
		assert.Equal(t, "Bearer private-fixture-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/directory/room/"):
			if f.resolveError || (f.unresolvedRoom != "" && strings.Contains(r.URL.Path, f.unresolvedRoom)) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprint(w, `{"errcode":"M_NOT_FOUND","error":"private-fixture-token private error text"}`)
				return
			}
			assert.Equal(t, "/_matrix/client/v3/directory/room/#"+exportRoom+":test.local", r.URL.Path)
			_, _ = fmt.Fprint(w, `{"room_id":"!fixture:test.local"}`)
		case strings.HasSuffix(r.URL.Path, "/aliases"):
			_ = json.NewEncoder(w).Encode(map[string]any{"aliases": f.aliases})
		case r.URL.Path == "/_synapse/admin/v1/rooms/!fixture:test.local/messages":
			assert.Equal(t, "b", r.URL.Query().Get("dir"))
			from := r.URL.Query().Get("from")
			f.requestedFrom = append(f.requestedFrom, from)
			page, ok := f.pages[from]
			if !ok {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = fmt.Fprint(w, `{"error":"private-fixture-token private error text"}`)
				return
			}
			_, _ = fmt.Fprint(w, page)
		default:
			t.Errorf("unexpected endpoint (potential side effect): %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func runExportFixture(t *testing.T, fixture *exportHTTPFixture) (string, error) {
	t.Helper()
	httpServer := fixture.serve(t)
	t.Cleanup(httpServer.Close)
	dir := t.TempDir()
	rooms := filepath.Join(dir, "rooms.json")
	requestedRooms := fixture.rooms
	if requestedRooms == nil {
		requestedRooms = []string{exportRoom}
	}
	input, err := json.Marshal(map[string]any{"schemaVersion": 1, "scope": "fixture", "roomIds": requestedRooms})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(rooms, input, 0o600))
	cfg := &config.Config{}
	cfg.Matrix.HomeserverURL = httpServer.URL
	cfg.Matrix.HomeserverName = "test.local"
	cfg.Matrix.AppServiceToken = "private-fixture-token"
	output := filepath.Join(dir, "events.jsonl")
	err = RunAttachmentsExport(context.Background(), cfg, []string{"--rooms", rooms, "--output", output})
	return output, err
}

func TestAttachmentsExport_RequestedRoomCoverageCannotHideUnresolvedRoom(t *testing.T) {
	fixture := &exportHTTPFixture{
		rooms: []string{exportRoom, exportOtherRoom}, unresolvedRoom: exportOtherRoom,
		aliases: []string{"#" + exportRoom + ":test.local"}, pages: map[string]string{"": `{"chunk":[]}`},
	}
	output, err := runExportFixture(t, fixture)
	require.Error(t, err)
	coverage := readExportCoverage(t, output)
	assert.Equal(t, fixture.rooms, coverage.RequestedRoomIDs)
	assert.Equal(t, []string{exportRoom}, coverage.ResolvedRoomIDs)
	require.Len(t, coverage.Rooms, 2)
	assert.True(t, coverage.Rooms[0].PaginationComplete)
	assert.False(t, coverage.Rooms[1].MappingComplete)
	assert.Equal(t, "alias_resolution_failed", coverage.Rooms[1].Failure)
	assert.False(t, coverage.Complete)
}

func TestAttachmentsExport_FilenamePolicy(t *testing.T) {
	assert.Equal(t, "a_b_c.png", exportDisplayName(" a/b\\c\x01.png ", "fallback"))
	assert.Equal(t, strings.Repeat("😀", 128), exportDisplayName(strings.Repeat("😀", 200), "fallback"))
	assert.Equal(t, "fallback", exportDisplayName(" \x00 ", "fallback"))
	assert.Equal(t, "résumé + photo.png", exportDisplayName("résumé + photo.png", "fallback"))
}

func readExportCoverage(t *testing.T, output string) attachmentExportCoverage {
	t.Helper()
	body := readPrivateExport(t, output+".coverage.json")
	assert.NotContains(t, string(body), "private-fixture-token")
	assert.NotContains(t, string(body), "private error text")
	var coverage attachmentExportCoverage
	require.NoError(t, json.Unmarshal(body, &coverage))
	return coverage
}

func readPrivateExport(t *testing.T, path string) []byte {
	t.Helper()
	//nolint:gosec // Test-generated path inside t.TempDir, never event or external input.
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return body
}

func TestAttachmentsExport_RootAndReplyPaginationPrivateOutput(t *testing.T) {
	fixture := &exportHTTPFixture{
		aliases: []string{"#unrelated:test.local", "#" + exportRoom + ":test.local"},
		pages: map[string]string{
			"":         `{"chunk":[{"event_id":"$reply","room_id":"!fixture:test.local","sender":"@` + exportActor + `:test.local","type":"m.room.message","origin_server_ts":200,"content":{"msgtype":"m.image","body":"PRIVATE CAPTION","filename":"résumé + photo.png","url":"mxc://test.local/media-reply","io.alkemio.document_id":"` + exportDocument + `","m.relates_to":{"rel_type":"m.thread","event_id":"$root","m.in_reply_to":{"event_id":"$previous"}}}},{"event_id":"$text","type":"m.room.message","content":{"msgtype":"m.text","body":"PRIVATE FULL MESSAGE"}}],"end":"page-two"}`,
			"page-two": `{"chunk":[{"event_id":"$root","sender":"@` + exportActor + `:test.local","type":"m.room.message","origin_server_ts":100,"content":{"msgtype":"m.file","body":"report.pdf","url":"mxc://test.local/media-root"}}]}`,
		},
	}
	output, err := runExportFixture(t, fixture)
	require.NoError(t, err)
	body := readPrivateExport(t, output)
	assert.NotContains(t, string(body), "PRIVATE")
	assert.NotContains(t, string(body), "private-fixture-token")
	assert.NotContains(t, string(body), "externalID")
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	var rows []attachmentExportEvent
	for scanner.Scan() {
		var row attachmentExportEvent
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &row))
		rows = append(rows, row)
	}
	require.NoError(t, scanner.Err())
	require.Len(t, rows, 2)
	assert.Equal(t, exportRoom, rows[0].RoomID)
	assert.Equal(t, "!fixture:test.local", rows[0].MatrixRoomID)
	assert.Equal(t, "media-reply", rows[0].MediaID)
	require.NotNil(t, rows[0].SenderID)
	assert.Equal(t, exportActor, *rows[0].SenderID)
	require.NotNil(t, rows[0].ThreadID)
	assert.Equal(t, "$root", *rows[0].ThreadID)
	assert.Equal(t, "résumé + photo.png", rows[0].DisplayName)
	assert.Equal(t, int64(200), rows[0].Timestamp)
	assert.Nil(t, rows[1].ThreadID)
	assert.Equal(t, []string{"", "page-two"}, fixture.requestedFrom)
	coverage := readExportCoverage(t, output)
	assert.True(t, coverage.Complete)
	assert.Equal(t, "fixture", coverage.Scope)
	assert.Equal(t, "test.local", coverage.Homeserver)
	assert.Equal(t, []string{exportRoom}, coverage.RequestedRoomIDs)
	assert.Equal(t, []string{exportRoom}, coverage.ResolvedRoomIDs)
	require.Len(t, coverage.Rooms, 1)
	assert.True(t, coverage.Rooms[0].MappingComplete)
	assert.True(t, coverage.Rooms[0].PaginationComplete)
	assert.Equal(t, 2, coverage.Rooms[0].Pages)
	assert.Equal(t, 3, coverage.Rooms[0].EventsScanned)
	assert.Equal(t, 2, coverage.Rooms[0].AttachmentsExported)
	for _, path := range []string{output, output + ".coverage.json"} {
		info, statErr := os.Stat(path)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestAttachmentsExport_IncompleteCoverageFailsWithoutReconciliation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fixture exportHTTPFixture
		reason  string
	}{
		{name: "unresolved", fixture: exportHTTPFixture{resolveError: true}, reason: "alias_resolution_failed"},
		{name: "conflicting aliases", fixture: exportHTTPFixture{aliases: []string{"#" + exportRoom + ":test.local", "#" + exportOtherRoom + ":test.local"}}, reason: "alias_mapping_failed"},
		{name: "wrong mapping", fixture: exportHTTPFixture{aliases: []string{"#" + exportOtherRoom + ":test.local"}}, reason: "alias_mapping_mismatch"},
		{name: "history failure", fixture: exportHTTPFixture{aliases: []string{"#" + exportRoom + ":test.local"}}, reason: "history_read_failed"},
		{name: "pagination cycle", fixture: exportHTTPFixture{aliases: []string{"#" + exportRoom + ":test.local"}, pages: map[string]string{"": `{"chunk":[],"end":"again"}`, "again": `{"chunk":[],"end":"again"}`}}, reason: "pagination_cycle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output, err := runExportFixture(t, &tc.fixture)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private-fixture-token")
			coverage := readExportCoverage(t, output)
			assert.False(t, coverage.Complete)
			require.Len(t, coverage.Rooms, 1)
			assert.Equal(t, tc.reason, coverage.Rooms[0].Failure)
			assert.False(t, coverage.Rooms[0].PaginationComplete)
		})
	}
}

func TestAttachmentsExport_RedactedAndUnmappableMediaAreExplicit(t *testing.T) {
	fixture := &exportHTTPFixture{
		aliases: []string{"#" + exportRoom + ":test.local"},
		pages: map[string]string{"": `{"chunk":[
			{"event_id":"$redacted","type":"m.room.message","unsigned":{"redacted_because":{"event_id":"$redaction","type":"m.room.redaction"}},"content":{}},
			{"event_id":"$foreign","type":"m.room.message","content":{"msgtype":"m.image","url":"mxc://elsewhere/media","body":"foreign.png"}},
			{"event_id":"$encrypted","type":"m.room.encrypted","content":{"ciphertext":"PRIVATE"}},
			{"event_id":"$unknown-sender","sender":"@unknown:test.local","type":"m.sticker","content":{"url":"mxc://test.local/sticker","body":"sticker"}}
		]}`},
	}
	output, err := runExportFixture(t, fixture)
	require.NoError(t, err, "history coverage may complete with explicit unsafe skipped markers")
	coverage := readExportCoverage(t, output)
	assert.True(t, coverage.Complete)
	assert.Equal(t, map[string]int{"redacted": 1, "unresolved_media": 1, "encrypted": 1, "unmapped_sender": 1}, coverage.Rooms[0].SkippedByReason)
	body := readPrivateExport(t, output)
	var row attachmentExportEvent
	require.NoError(t, json.Unmarshal(body, &row))
	assert.Equal(t, "sticker", row.MediaID)
	assert.Nil(t, row.SenderID)
	assert.NotContains(t, string(body), "PRIVATE")
}

func TestAttachmentsExport_RejectsInvalidScopeAndExistingOutput(t *testing.T) {
	dir := t.TempDir()
	rooms, output := filepath.Join(dir, "rooms.json"), filepath.Join(dir, "events.jsonl")
	cfg := &config.Config{}
	cfg.Matrix.HomeserverName, cfg.Matrix.HomeserverURL = "test.local", "http://127.0.0.1:1"
	cfg.Matrix.AppServiceToken = "private-fixture-token"
	for _, input := range []string{
		`{"schemaVersion":1,"scope":"production","roomIds":["` + exportRoom + `"]}`,
		`{"schemaVersion":1,"scope":"fixture","roomIds":["` + exportRoom + `","` + exportRoom + `"]}`,
		`{"schemaVersion":1,"scope":"fixture","roomIds":["display name"]}`,
	} {
		require.NoError(t, os.WriteFile(rooms, []byte(input), 0o600))
		require.Error(t, RunAttachmentsExport(context.Background(), cfg, []string{"--rooms", rooms, "--output", output}))
		_, err := os.Stat(output)
		assert.True(t, os.IsNotExist(err))
	}
	require.NoError(t, os.WriteFile(rooms, []byte(`{"schemaVersion":1,"scope":"fixture","roomIds":["`+exportRoom+`"]}`), 0o600))
	require.NoError(t, os.WriteFile(output, []byte("keep existing report"), 0o600))
	require.Error(t, RunAttachmentsExport(context.Background(), cfg, []string{"--rooms", rooms, "--output", output}))
	contents := readPrivateExport(t, output)
	assert.Equal(t, "keep existing report", string(contents))
}

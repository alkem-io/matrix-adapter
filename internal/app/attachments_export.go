package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/config"
	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/infrastructure/matrix"
)

type attachmentExportInput struct {
	SchemaVersion int      `json:"schemaVersion"`
	Scope         string   `json:"scope"`
	RoomIDs       []string `json:"roomIds"`
}

type attachmentExportEvent struct {
	RoomID       string  `json:"roomId"`
	MatrixRoomID string  `json:"matrixRoomId"`
	EventID      string  `json:"eventId"`
	SenderID     *string `json:"senderId"`
	MediaID      string  `json:"mediaId"`
	DisplayName  string  `json:"displayName"`
	ThreadID     *string `json:"threadId"`
	Timestamp    int64   `json:"timestamp"`
}

type attachmentRoomCoverage struct {
	RoomID              string         `json:"roomId"`
	MatrixRoomID        string         `json:"matrixRoomId,omitempty"`
	MappingComplete     bool           `json:"mappingComplete"`
	PaginationComplete  bool           `json:"paginationComplete"`
	Pages               int            `json:"pages"`
	EventsScanned       int            `json:"eventsScanned"`
	AttachmentsExported int            `json:"attachmentsExported"`
	SkippedByReason     map[string]int `json:"skippedByReason"`
	Failure             string         `json:"failure,omitempty"`
}

type attachmentExportCoverage struct {
	SchemaVersion    int                      `json:"schemaVersion"`
	Scope            string                   `json:"scope"`
	Homeserver       string                   `json:"homeserver"`
	RequestedRoomIDs []string                 `json:"requestedRoomIds"`
	ResolvedRoomIDs  []string                 `json:"resolvedRoomIds"`
	StartedAt        time.Time                `json:"startedAt"`
	CompletedAt      time.Time                `json:"completedAt"`
	Complete         bool                     `json:"complete"`
	Failure          string                   `json:"failure,omitempty"`
	Rooms            []attachmentRoomCoverage `json:"rooms"`
}

type attachmentExportReader struct {
	client *mautrix.Client
	admin  *matrix.SynapseAdmin
	mapper *domain.IDMapper
}

// RunAttachmentsExport reads only explicitly requested rooms, producing private
// JSONL and a coverage sidecar. It does not start App, register a user, elevate
// the bot, join rooms, reconcile aliases, or connect to RabbitMQ/the database.
// Complete means every requested history was scanned, not that the operator's
// requested set covers production. The server must check its room inventory and
// unsafe skipped markers before permitting normalization.
func RunAttachmentsExport(ctx context.Context, cfg *config.Config, args []string) error {
	input, output, err := parseAttachmentExportArgs(args)
	if err != nil {
		return err
	}
	reader, err := newAttachmentExportReader(cfg)
	if err != nil {
		return err
	}
	eventsFile, coverageFile, err := createAttachmentExportFiles(output)
	if err != nil {
		return err
	}
	defer func() { _ = eventsFile.Close() }()
	defer func() { _ = coverageFile.Close() }()
	coverage := attachmentExportCoverage{
		SchemaVersion: 1, Scope: input.Scope, Homeserver: cfg.Matrix.HomeserverName,
		RequestedRoomIDs: input.RoomIDs, ResolvedRoomIDs: []string{},
		StartedAt: time.Now().UTC(), Complete: true,
		Rooms: make([]attachmentRoomCoverage, 0, len(input.RoomIDs)),
	}
	encoder := json.NewEncoder(eventsFile)
	for _, room := range input.RoomIDs {
		result := reader.exportRoom(mautrix.WithMaxRetries(ctx, 0), room, encoder)
		coverage.Rooms = append(coverage.Rooms, result)
		if result.MappingComplete {
			coverage.ResolvedRoomIDs = append(coverage.ResolvedRoomIDs, room)
		}
		if !result.PaginationComplete {
			coverage.Complete = false
		}
	}
	coverage.CompletedAt = time.Now().UTC()
	if err := eventsFile.Sync(); err != nil {
		coverage.Complete = false
		coverage.Failure = "event_output_sync_failed"
	}
	if err := json.NewEncoder(coverageFile).Encode(coverage); err != nil {
		return errors.New("cannot write attachment coverage report")
	}
	if err := coverageFile.Sync(); err != nil {
		return errors.New("cannot persist attachment coverage report")
	}
	if !coverage.Complete {
		return errors.New("attachment export incomplete; inspect coverage sidecar")
	}
	return nil
}

func parseAttachmentExportArgs(args []string) (*attachmentExportInput, string, error) {
	flags := flag.NewFlagSet("attachments-export", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	rooms := flags.String("rooms", "", "explicit room inventory JSON")
	output := flags.String("output", "", "new private JSONL output path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *rooms == "" || *output == "" {
		return nil, "", errors.New("usage: attachments-export --rooms <room-list.json> --output <events.jsonl>")
	}
	file, err := os.Open(*rooms)
	if err != nil {
		return nil, "", errors.New("cannot read room inventory")
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(io.LimitReader(file, 16<<20))
	decoder.DisallowUnknownFields()
	var input attachmentExportInput
	if err := decoder.Decode(&input); err != nil {
		return nil, "", errors.New("invalid room inventory JSON")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, "", errors.New("room inventory must contain one JSON object")
	}
	if err := validateAttachmentExportInput(&input); err != nil {
		return nil, "", err
	}
	return &input, *output, nil
}

func validateAttachmentExportInput(input *attachmentExportInput) error {
	if input.SchemaVersion != 1 || (input.Scope != "fixture" && input.Scope != "supported-room-inventory") || len(input.RoomIDs) == 0 {
		return errors.New("room inventory requires schemaVersion 1, explicit scope and roomIds")
	}
	seen := make(map[uuid.UUID]bool, len(input.RoomIDs))
	for i, raw := range input.RoomIDs {
		room, parseErr := uuid.Parse(raw)
		if parseErr != nil || room == uuid.Nil || seen[room] {
			return errors.New("room inventory contains an invalid or duplicate room UUID")
		}
		seen[room], input.RoomIDs[i] = true, room.String()
	}
	return nil
}

func newAttachmentExportReader(cfg *config.Config) (*attachmentExportReader, error) {
	if cfg == nil || cfg.Matrix.HomeserverName == "" || cfg.Matrix.HomeserverURL == "" || cfg.Matrix.AppServiceToken == "" {
		return nil, errors.New("export requires configured homeserver name, URL and appservice token")
	}
	client, err := mautrix.NewClient(cfg.Matrix.HomeserverURL, "", cfg.Matrix.AppServiceToken)
	if err != nil {
		return nil, errors.New("cannot initialize read-only Matrix client")
	}
	admin, err := matrix.NewSynapseAdmin(cfg.Matrix.HomeserverURL, cfg.Matrix.AppServiceToken)
	if err != nil {
		return nil, errors.New("cannot initialize read-only Matrix history client")
	}
	return &attachmentExportReader{client: client, admin: admin, mapper: domain.NewIDMapper(cfg.Matrix.HomeserverName)}, nil
}

func createAttachmentExportFiles(output string) (*os.File, *os.File, error) {
	//nolint:gosec // Operator-selected output, exclusive create prevents overwriting files/symlinks; mode is private.
	eventsFile, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, nil, errors.New("cannot exclusively create private event output")
	}
	//nolint:gosec // Same operator-selected directory and exclusive/private creation as the event output.
	coverageFile, err := os.OpenFile(output+".coverage.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = eventsFile.Close()
		_ = os.Remove(output)
		return nil, nil, errors.New("cannot exclusively create private coverage output")
	}
	return eventsFile, coverageFile, nil
}

func (r *attachmentExportReader) exportRoom(ctx context.Context, room string, out *json.Encoder) attachmentRoomCoverage {
	coverage := attachmentRoomCoverage{RoomID: room, SkippedByReason: map[string]int{}}
	roomID, failure := r.resolveRoom(ctx, uuid.MustParse(room))
	if failure != "" {
		coverage.Failure = failure
		return coverage
	}
	coverage.MatrixRoomID, coverage.MappingComplete = roomID.String(), true
	from := ""
	seenTokens := map[string]bool{"": true}
	seenEvents := map[id.EventID]bool{}
	for {
		requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		page, err := r.admin.GetRoomMessages(requestCtx, roomID, from, "b", 100)
		cancel()
		if err != nil || page == nil {
			coverage.Failure = "history_read_failed"
			return coverage
		}
		coverage.Pages++
		for _, evt := range page.Chunk {
			coverage.EventsScanned++
			if evt == nil || !validExportEventID(evt.ID.String()) || (evt.RoomID != "" && evt.RoomID != roomID) {
				coverage.SkippedByReason["invalid_event"]++
				continue
			}
			if seenEvents[evt.ID] {
				coverage.SkippedByReason["duplicate_event"]++
				continue
			}
			seenEvents[evt.ID] = true
			row := r.mediaEvent(evt, &coverage)
			if row != nil {
				if err := out.Encode(row); err != nil {
					coverage.Failure = "event_output_failed"
					return coverage
				}
				coverage.AttachmentsExported++
			}
		}
		if page.End == "" {
			coverage.PaginationComplete = true
			return coverage
		}
		if seenTokens[page.End] {
			coverage.Failure = "pagination_cycle"
			return coverage
		}
		seenTokens[page.End], from = true, page.End
	}
}

func (r *attachmentExportReader) resolveRoom(ctx context.Context, room uuid.UUID) (id.RoomID, string) {
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resolved, err := r.client.ResolveAlias(requestCtx, id.RoomAlias(r.mapper.RoomAlias(room)))
	if err != nil || resolved == nil || resolved.RoomID == "" {
		return "", "alias_resolution_failed"
	}
	aliases, err := r.client.GetAliases(requestCtx, resolved.RoomID)
	if err != nil || aliases == nil {
		return "", "alias_mapping_failed"
	}
	names := make([]string, len(aliases.Aliases))
	for i, alias := range aliases.Aliases {
		names[i] = string(alias)
	}
	mapped, err := matrix.AlkemioRoomIDFromAliases(names, r.mapper)
	if err != nil {
		return "", "alias_mapping_failed"
	}
	if mapped != room {
		return "", "alias_mapping_mismatch"
	}
	return resolved.RoomID, ""
}

func (r *attachmentExportReader) mediaEvent(evt *event.Event, coverage *attachmentRoomCoverage) *attachmentExportEvent {
	if evt.Unsigned.RedactedBecause != nil {
		coverage.SkippedByReason["redacted"]++
		return nil
	}
	if evt.Type == event.EventEncrypted {
		coverage.SkippedByReason["encrypted"]++
		return nil
	}
	if !isExportMedia(evt) {
		return nil
	}
	attachment := matrix.AttachmentForExport(evt, r.mapper)
	if attachment == nil || attachment.MediaID == "" {
		coverage.SkippedByReason["unresolved_media"]++
		return nil
	}
	if !validExportMediaID(attachment.MediaID) {
		coverage.SkippedByReason["invalid_media_id"]++
		return nil
	}
	row := &attachmentExportEvent{
		RoomID: coverage.RoomID, MatrixRoomID: coverage.MatrixRoomID,
		EventID: evt.ID.String(), MediaID: attachment.MediaID,
		DisplayName: exportDisplayName(attachment.DisplayName, attachment.MediaID), Timestamp: evt.Timestamp,
	}
	actor := r.mapper.AlkemioActorID(evt.Sender)
	if actor == uuid.Nil || evt.Sender.Homeserver() != r.mapper.HomeserverDomain() {
		coverage.SkippedByReason["unmapped_sender"]++
	} else {
		value := actor.String()
		row.SenderID = &value
	}

	row.ThreadID = exportThreadID(evt, coverage)
	return row
}

func exportThreadID(evt *event.Event, coverage *attachmentRoomCoverage) *string {
	if relation, ok := evt.Content.Raw["m.relates_to"].(map[string]any); ok && relation["rel_type"] == "m.thread" {
		if parent, ok := relation["event_id"].(string); ok && validExportEventID(parent) {
			return &parent
		}
		coverage.SkippedByReason["invalid_thread_id"]++
	}
	return nil
}

func validExportEventID(value string) bool {
	return strings.HasPrefix(value, "$") && len(value) <= 512 && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validExportMediaID(value string) bool {
	if len(value) == 0 || len(value) > 256 {
		return false
	}
	for _, c := range value {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

// exportDisplayName matches the server/file-service filename policy: remove
// controls, replace separators, trim, and cap UTF-8 bytes without splitting a rune.
func exportDisplayName(name, fallback string) string {
	cleaned := strings.TrimSpace(strings.Map(func(c rune) rune {
		if unicode.IsControl(c) {
			return -1
		}
		if c == '/' || c == '\\' {
			return '_'
		}
		return c
	}, name))
	if len(cleaned) > 512 {
		end := 512
		for !utf8.RuneStart(cleaned[end]) {
			end--
		}
		cleaned = cleaned[:end]
	}
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return fallback
	}
	return cleaned
}

func isExportMedia(evt *event.Event) bool {
	if evt.Type == event.EventSticker {
		return true
	}
	if evt.Type != event.EventMessage {
		return false
	}
	switch evt.Content.Raw["msgtype"] {
	case "m.image", "m.video", "m.audio", "m.file":
		return true
	default:
		return false
	}
}

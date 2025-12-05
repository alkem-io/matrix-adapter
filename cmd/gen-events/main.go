// Package main provides a tool to generate TypeScript event definitions from Go code.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: gen-events <output-dir>")
		fmt.Println("  Generates matrix.adapter.event.type.ts and commands.ts in the output directory")
		os.Exit(1)
	}
	outputDir := os.Args[1]

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0750); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// 1. Parse topics.go for topic constants
	topicEvents, err := parseTopicConstants("internal/infrastructure/queue/topics.go")
	if err != nil {
		fmt.Printf("Error parsing topics.go: %v\n", err)
		os.Exit(1)
	}

	// 2. Parse event_service.go for outgoing events
	outgoingEvents, err := parseOutgoingEvents("internal/core/service/event_service.go")
	if err != nil {
		fmt.Printf("Error parsing event_service.go: %v\n", err)
		os.Exit(1)
	}

	// Combine events for enum generation
	allEvents := make(map[string]string) // value -> key
	for _, evt := range topicEvents {
		allEvents[evt] = generateEnumKey(evt)
	}
	for _, evt := range outgoingEvents {
		allEvents[evt] = generateEnumKey(evt)
	}

	// 3. Read existing TS file to preserve custom mappings if any
	eventTypeFile := filepath.Join(outputDir, "matrix.adapter.event.type.ts")
	existingMapping := parseExistingTSFile(eventTypeFile)
	for val, key := range existingMapping {
		if _, ok := allEvents[val]; ok {
			allEvents[val] = key
		}
	}

	// 4. Generate event type enum TS content
	eventContent := generateTSContent(allEvents)

	// 5. Write event type file
	err = os.WriteFile(eventTypeFile, []byte(eventContent), 0600)
	if err != nil {
		fmt.Printf("Error writing event type file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Successfully generated %s with %d events\n", eventTypeFile, len(allEvents))

	// 6. Parse command registry from Go
	commands, err := parseCommandRegistry("pkg/dto/commands.go")
	if err != nil {
		fmt.Printf("Error parsing commands.go: %v\n", err)
		os.Exit(1)
	}

	// 7. Generate commands.ts
	commandsFile := filepath.Join(outputDir, "commands.ts")
	commandsContent := generateCommandsTS(commands)
	err = os.WriteFile(commandsFile, []byte(commandsContent), 0600)
	if err != nil {
		fmt.Printf("Error writing commands file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Successfully generated %s with %d commands\n", commandsFile, len(commands))
}

// CommandDef mirrors the Go struct for parsing
type CommandDef struct {
	Topic        string
	RequestType  string
	ResponseType string
}

// parseCommandRegistry parses the CommandRegistry and OutgoingEventRegistry slices
// from the Go source file using AST parsing for reliability.
func parseCommandRegistry(path string) ([]CommandDef, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	// First pass: collect all const string values
	constants := make(map[string]string)
	ast.Inspect(
		node, func(n ast.Node) bool {
			genDecl, ok := n.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.CONST {
				return true
			}
			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range valueSpec.Names {
					if i < len(valueSpec.Values) {
						if lit, ok := valueSpec.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
							constants[name.Name] = strings.Trim(lit.Value, "\"")
						}
					}
				}
			}
			return true
		},
	)

	// Second pass: extract command definitions
	var commands []CommandDef
	ast.Inspect(
		node, func(n ast.Node) bool {
			genDecl, ok := n.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.VAR {
				return true
			}

			commands = append(commands, extractCommandsFromGenDecl(genDecl, constants)...)
			return true
		},
	)

	return commands, nil
}

// extractCommandsFromGenDecl extracts CommandDef entries from a var declaration.
func extractCommandsFromGenDecl(genDecl *ast.GenDecl, constants map[string]string) []CommandDef {
	var commands []CommandDef

	for _, spec := range genDecl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		commands = append(commands, extractCommandsFromValueSpec(valueSpec, constants)...)
	}

	return commands
}

// extractCommandsFromValueSpec extracts CommandDef entries from a value spec.
func extractCommandsFromValueSpec(valueSpec *ast.ValueSpec, constants map[string]string) []CommandDef {
	var commands []CommandDef

	for i, name := range valueSpec.Names {
		if name.Name != "CommandRegistry" && name.Name != "OutgoingEventRegistry" {
			continue
		}
		if i >= len(valueSpec.Values) {
			continue
		}

		compLit, ok := valueSpec.Values[i].(*ast.CompositeLit)
		if !ok {
			continue
		}

		for _, elt := range compLit.Elts {
			if cmd := parseCommandDefLiteral(elt, constants); cmd != nil {
				commands = append(commands, *cmd)
			}
		}
	}

	return commands
}

// parseCommandDefLiteral extracts CommandDef fields from an AST composite literal.
func parseCommandDefLiteral(expr ast.Expr, constants map[string]string) *CommandDef {
	compLit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}

	cmd := &CommandDef{}

	for _, elt := range compLit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		extractCommandField(kv, cmd, constants)
	}

	if cmd.Topic != "" && cmd.ResponseType != "" {
		return cmd
	}
	return nil
}

// extractCommandField extracts a single field from a key-value expression.
func extractCommandField(kv *ast.KeyValueExpr, cmd *CommandDef, constants map[string]string) {
	key, ok := kv.Key.(*ast.Ident)
	if !ok {
		return
	}

	var strVal string

	switch v := kv.Value.(type) {
	case *ast.BasicLit:
		// Direct string literal: Topic: "communication.room.create"
		if v.Kind != token.STRING {
			return
		}
		strVal = strings.Trim(v.Value, "\"")
	case *ast.Ident:
		// Constant reference: Topic: TopicRoomCreate
		resolved, ok := constants[v.Name]
		if !ok {
			fmt.Fprintf(os.Stderr, "Warning: unresolved constant %s\n", v.Name)
			return
		}
		strVal = resolved
	default:
		return
	}

	switch key.Name {
	case "Topic":
		cmd.Topic = strVal
	case "RequestType":
		cmd.RequestType = strVal
	case "ResponseType":
		cmd.ResponseType = strVal
	}
}

func generateCommandsTS(commands []CommandDef) string {
	var sb strings.Builder

	sb.WriteString("// Code generated by gen-events. DO NOT EDIT.\n\n")
	sb.WriteString("import type {\n")

	// Collect unique types for imports
	types := make(map[string]bool)
	for _, cmd := range commands {
		if cmd.RequestType != "" {
			types[cmd.RequestType] = true
		}
		types[cmd.ResponseType] = true
	}

	// Sort for deterministic output
	typeList := make([]string, 0, len(types))
	for t := range types {
		typeList = append(typeList, t)
	}
	sort.Strings(typeList)

	for _, t := range typeList {
		sb.WriteString(fmt.Sprintf("  %s,\n", t))
	}
	sb.WriteString("} from './dto';\n\n")

	// Generate Commands object
	sb.WriteString("/**\n")
	sb.WriteString(" * Command registry mapping topics to their request/response types.\n")
	sb.WriteString(" * Use RequestFor<T> and ResponseFor<T> type helpers for type-safe access.\n")
	sb.WriteString(" */\n")
	sb.WriteString("export const Commands = {\n")

	// Separate commands (have request type) from events (no request type)
	var cmdList []CommandDef
	var eventList []CommandDef
	for _, cmd := range commands {
		if cmd.RequestType != "" {
			cmdList = append(cmdList, cmd)
		} else {
			eventList = append(eventList, cmd)
		}
	}

	// Sort commands by topic
	sort.Slice(
		cmdList, func(i, j int) bool {
			return cmdList[i].Topic < cmdList[j].Topic
		},
	)

	for _, cmd := range cmdList {
		sb.WriteString(fmt.Sprintf("  '%s': {\n", cmd.Topic))
		sb.WriteString(fmt.Sprintf("    request: {} as %s,\n", cmd.RequestType))
		sb.WriteString(fmt.Sprintf("    response: {} as %s,\n", cmd.ResponseType))
		sb.WriteString("  },\n")
	}

	sb.WriteString("} as const;\n\n")

	// Generate OutgoingEvents object
	if len(eventList) > 0 {
		sb.WriteString("/**\n")
		sb.WriteString(" * Outgoing events emitted by the adapter (no request, only payload).\n")
		sb.WriteString(" */\n")
		sb.WriteString("export const OutgoingEvents = {\n")

		sort.Slice(
			eventList, func(i, j int) bool {
				return eventList[i].Topic < eventList[j].Topic
			},
		)

		for _, evt := range eventList {
			sb.WriteString(fmt.Sprintf("  '%s': {\n", evt.Topic))
			sb.WriteString(fmt.Sprintf("    payload: {} as %s,\n", evt.ResponseType))
			sb.WriteString("  },\n")
		}

		sb.WriteString("} as const;\n\n")
	}

	// Generate type helpers
	sb.WriteString("// ============================================================================\n")
	sb.WriteString("// Type Helpers\n")
	sb.WriteString("// ============================================================================\n\n")

	sb.WriteString("/** All command topics */\n")
	sb.WriteString("export type CommandTopic = keyof typeof Commands;\n\n")

	sb.WriteString("/** Get the request type for a given command topic */\n")
	sb.WriteString("export type RequestFor<T extends CommandTopic> = (typeof Commands)[T]['request'];\n\n")

	sb.WriteString("/** Get the response type for a given command topic */\n")
	sb.WriteString("export type ResponseFor<T extends CommandTopic> = (typeof Commands)[T]['response'];\n\n")

	if len(eventList) > 0 {
		sb.WriteString("/** All outgoing event topics */\n")
		sb.WriteString("export type OutgoingEventTopic = keyof typeof OutgoingEvents;\n\n")

		sb.WriteString("/** Get the payload type for a given outgoing event topic */\n")
		sb.WriteString("export type PayloadFor<T extends OutgoingEventTopic> = (typeof OutgoingEvents)[T]['payload'];\n")
	}

	return sb.String()
}

func parseTopicConstants(path string) ([]string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var events []string
	ast.Inspect(
		node, func(n ast.Node) bool {
			genDecl, ok := n.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.CONST {
				return true
			}
			events = append(events, extractStringConstants(genDecl)...)
			return true
		},
	)

	return events, nil
}

// extractStringConstants extracts string constant values from a const declaration.
func extractStringConstants(genDecl *ast.GenDecl) []string {
	var constants []string

	for _, spec := range genDecl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		for _, value := range valueSpec.Values {
			lit, ok := value.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			constants = append(constants, strings.Trim(lit.Value, "\""))
		}
	}

	return constants
}

func parseOutgoingEvents(path string) ([]string, error) {
	//nolint:gosec // CLI tool reading known path
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Regex to find s.queue.Publish("event.name", ...)
	// This is a simple heuristic
	re := regexp.MustCompile(`Publish\("([^"]+)"`)
	matches := re.FindAllStringSubmatch(string(content), -1)

	var events []string
	for _, m := range matches {
		if len(m) > 1 {
			events = append(events, m[1])
		}
	}
	return events, nil
}

func parseExistingTSFile(path string) map[string]string {
	mapping := make(map[string]string)
	//nolint:gosec // CLI tool reading user provided path
	content, err := os.ReadFile(path)
	if err != nil {
		return mapping // Return empty if file doesn't exist
	}

	// Regex to find KEY = 'value'
	re := regexp.MustCompile(`([A-Z_]+)\s*=\s*'([^']+)'`)
	matches := re.FindAllStringSubmatch(string(content), -1)

	for _, m := range matches {
		if len(m) > 2 {
			mapping[m[2]] = m[1]
		}
	}
	return mapping
}

func generateEnumKey(value string) string {
	// Convert camelCase or dot.separated to UPPER_SNAKE_CASE

	// First replace dots with underscores
	s := strings.ReplaceAll(value, ".", "_")

	// Then handle camelCase
	var result strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(rune(s[i-1])) || (i+1 < len(s) && unicode.IsLower(rune(s[i+1])))) {
			result.WriteRune('_')
		}
		result.WriteRune(unicode.ToUpper(r))
	}

	return result.String()
}

func generateTSContent(events map[string]string) string {
	keys := make([]string, 0, len(events))
	for val := range events {
		keys = append(keys, val)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("export enum MatrixAdapterEventType {\n\n")

	// We can't easily group them without more logic, so we'll just list them sorted by Key or Value
	// Let's sort by Enum Key for better readability
	type pair struct {
		Key   string
		Value string
	}
	pairs := make([]pair, 0, len(keys))
	for _, val := range keys {
		pairs = append(pairs, pair{Key: events[val], Value: val})
	}

	sort.Slice(
		pairs, func(i, j int) bool {
			return pairs[i].Key < pairs[j].Key
		},
	)

	for _, p := range pairs {
		sb.WriteString(fmt.Sprintf("  %s = '%s',\n", p.Key, p.Value))
	}

	sb.WriteString("\n}\n")
	return sb.String()
}

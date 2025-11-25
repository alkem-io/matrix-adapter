// Package main provides a tool to generate TypeScript event definitions from Go code.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: gen-events <output-ts-file>")
		os.Exit(1)
	}
	outputFile := os.Args[1]

	// 1. Parse router.go for incoming events
	routerEvents, err := parseRouterEvents("internal/infrastructure/queue/router.go")
	if err != nil {
		fmt.Printf("Error parsing router.go: %v\n", err)
		os.Exit(1)
	}

	// 2. Parse event_service.go for outgoing events
	outgoingEvents, err := parseOutgoingEvents("internal/core/service/event_service.go")
	if err != nil {
		fmt.Printf("Error parsing event_service.go: %v\n", err)
		os.Exit(1)
	}

	// Combine events
	allEvents := make(map[string]string) // value -> key
	for _, evt := range routerEvents {
		allEvents[evt] = generateEnumKey(evt)
	}
	for _, evt := range outgoingEvents {
		allEvents[evt] = generateEnumKey(evt)
	}

	// 3. Read existing TS file to preserve custom mappings if any
	existingMapping := parseExistingTSFile(outputFile)
	for val, key := range existingMapping {
		if _, ok := allEvents[val]; ok {
			allEvents[val] = key
		}
	}

	// 4. Generate TS content
	content := generateTSContent(allEvents)

	// 5. Write to file
	err = os.WriteFile(outputFile, []byte(content), 0600)
	if err != nil {
		fmt.Printf("Error writing output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated %s with %d events\n", outputFile, len(allEvents))
}

func parseRouterEvents(path string) ([]string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var events []string
	ast.Inspect(node, func(n ast.Node) bool {
		if extracted := extractEventsFromNode(n); len(extracted) > 0 {
			events = append(events, extracted...)
		}
		return true
	})

	return events, nil
}

func extractEventsFromNode(n ast.Node) []string {
	// Look for map[string]func... literal
	compLit, ok := n.(*ast.CompositeLit)
	if !ok {
		return nil
	}

	// Check if it's a map
	mapType, ok := compLit.Type.(*ast.MapType)
	if !ok {
		return nil
	}

	// Check key type is string
	if ident, ok := mapType.Key.(*ast.Ident); !ok || ident.Name != "string" {
		return nil
	}

	events := make([]string, 0, len(compLit.Elts))
	// Extract keys
	for _, elt := range compLit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		lit, ok := kv.Key.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		events = append(events, strings.Trim(lit.Value, "\""))
	}
	return events
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

	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Key < pairs[j].Key
	})

	for _, p := range pairs {
		sb.WriteString(fmt.Sprintf("  %s = '%s',\n", p.Key, p.Value))
	}

	sb.WriteString("\n}\n")
	return sb.String()
}

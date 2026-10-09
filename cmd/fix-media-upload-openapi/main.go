// Command fix-media-upload-openapi supplies the raw-body, 201-only contract
// missed by go-apispec v0.4.25. Only this internal upload operation is replaced;
// every other generated operation remains byte-for-byte unchanged.
package main

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

//go:embed operation.yaml
var mediaUploadOperation string

func main() {
	if err := correctFile(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func correctFile() error {
	directory, err := os.OpenRoot(".")
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	input, err := directory.ReadFile("openapi.yaml")
	if err != nil {
		return err
	}
	output, err := correctMediaUpload(input)
	if err != nil {
		return err
	}
	return directory.WriteFile("openapi.yaml", output, 0o600)
}

func correctMediaUpload(input []byte) ([]byte, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(input, &document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 {
		return nil, fmt.Errorf("expected one OpenAPI document")
	}
	node := document.Content[0]
	var key *yaml.Node
	for _, name := range []string{"paths", "/internal/media/upload", "post"} {
		key = nil
		if node.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("expected mapping containing %q", name)
		}
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Value == name {
				key = node.Content[i]
				node = node.Content[i+1]
				break
			}
		}
		if key == nil {
			return nil, fmt.Errorf("missing media-upload operation path component %q", name)
		}
	}
	// Use parser positions to replace just this generated block; re-encoding the
	// whole YAML would unnecessarily change formatting and unrelated operations.
	lines := strings.SplitAfter(string(input), "\n")
	start := key.Line - 1
	indent := strings.Repeat(" ", key.Column-1)
	if strings.TrimSpace(lines[start]) != "post:" {
		return nil, fmt.Errorf("expected generated block-style post operation")
	}
	end := start + 1
	for end < len(lines) {
		line := strings.TrimRight(lines[end], "\r\n")
		if strings.TrimSpace(line) != "" && len(line)-len(strings.TrimLeft(line, " ")) <= len(indent) {
			break
		}
		end++
	}
	var result strings.Builder
	result.WriteString(strings.Join(lines[:start], ""))
	result.WriteString(indent + "post:\n")
	for _, line := range strings.Split(strings.TrimSuffix(mediaUploadOperation, "\n"), "\n") {
		result.WriteString(indent + "  " + line + "\n")
	}
	result.WriteString(strings.Join(lines[end:], ""))
	return []byte(result.String()), nil
}

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	httpinfra "github.com/alkem-io/matrix-adapter/internal/infrastructure/http"
)

const generatedFixture = `openapi: 3.1.1
paths:
  /before:
    get:
      responses:
        "200": {description: preserved}
  /rest/messaging/media/upload:
    post:
      operationId: MediaUploadHandler.handleUpload
      responses:
        "200": {description: wrong inferred success}
        "201": {description: Created}
        "400": {description: Bad Request}
  /after:
    get:
      responses:
        "200": {description: also preserved}
`

func TestCorrectMediaUpload_OnlyOperationAndStable(t *testing.T) {
	result, err := correctMediaUpload([]byte(generatedFixture))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(generatedFixture, "    post:\n")
	end := strings.Index(generatedFixture, "  /after:\n")
	if !bytes.HasPrefix(result, []byte(generatedFixture[:start])) || !bytes.HasSuffix(result, []byte(generatedFixture[end:])) {
		t.Fatal("changed another operation")
	}
	var spec struct {
		Paths map[string]map[string]struct {
			RequestBody map[string]any `yaml:"requestBody"`
			Responses   map[string]any `yaml:"responses"`
		}
	}
	if err := yaml.Unmarshal(result, &spec); err != nil {
		t.Fatal(err)
	}
	operation := spec.Paths["/rest/messaging/media/upload"]["post"]
	if operation.RequestBody == nil || operation.Responses["201"] == nil || operation.Responses["200"] != nil {
		t.Fatalf("wrong upload contract: %+v", operation)
	}
	if !bytes.Contains(result, []byte("format: binary")) {
		t.Fatal("missing raw binary body")
	}
	again, err := correctMediaUpload(result)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result, again) {
		t.Fatal("correction not idempotent")
	}
}

func TestCorrectMediaUpload_RequiresExpectedOperation(t *testing.T) {
	for _, source := range []string{"paths: {}", strings.Replace(generatedFixture, "    post:", "    put:", 1)} {
		if _, err := correctMediaUpload([]byte(source)); err == nil {
			t.Fatal("missing operation silently accepted")
		}
	}
}

func TestMediaUploadContract_ActualSuccessFieldsAndStatusSet(t *testing.T) {
	var operation struct {
		Responses map[string]struct {
			Content map[string]struct {
				Schema struct {
					Required   []string
					Properties map[string]any
				}
			}
		}
	}
	if err := yaml.Unmarshal([]byte(mediaUploadOperation), &operation); err != nil {
		t.Fatal(err)
	}
	expected := []string{"201", "400", "401", "408", "411", "413", "502", "503", "504"}
	if len(operation.Responses) != len(expected) {
		t.Fatalf("unexpected status count: %v", operation.Responses)
	}
	for _, status := range expected {
		if _, ok := operation.Responses[status]; !ok {
			t.Fatalf("missing %s", status)
		}
	}
	encoded, err := json.Marshal(httpinfra.MediaUploadResult{ContentURI: "mxc://local.test/media", MediaID: "media"})
	if err != nil {
		t.Fatal(err)
	}
	var actual map[string]string
	if err = json.Unmarshal(encoded, &actual); err != nil {
		t.Fatal(err)
	}
	success := operation.Responses["201"].Content["application/json"].Schema
	if len(success.Required) != len(actual) || len(success.Properties) != len(actual) {
		t.Fatal("success fields drifted from Go result")
	}
	for _, field := range success.Required {
		if actual[field] == "" || success.Properties[field] == nil {
			t.Fatalf("success field %s drifted", field)
		}
	}
	if strings.Contains(mediaUploadOperation, "$ref:") {
		t.Fatal("operation must be self-contained without dangling refs")
	}
}

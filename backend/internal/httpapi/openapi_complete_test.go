package httpapi

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// TestOpenAPIOperationsAreComplete keeps the published API reference useful:
// every operation is identified, summarised, tagged with a declared tag, has
// a success response, documents 401/403 when secured and declares its path
// parameters.
func TestOpenAPIOperationsAreComplete(t *testing.T) {
	body, err := os.ReadFile("../../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Tags       []struct{ Name, Description string } `yaml:"tags"`
		Paths      map[string]map[string]any            `yaml:"paths"`
		Components struct {
			Parameters map[string]map[string]any `yaml:"parameters"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(body, &spec); err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	for _, tag := range spec.Tags {
		if tag.Description == "" {
			t.Errorf("tag %s has no description", tag.Name)
		}
		declared[tag.Name] = true
	}
	ids := map[string]string{}
	pathParam := regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)
	for path, item := range spec.Paths {
		for method, raw := range item {
			if !strings.Contains("get post put patch delete", method) {
				continue
			}
			where := strings.ToUpper(method) + " " + path
			op, ok := raw.(map[string]any)
			if !ok {
				t.Errorf("%s is not an operation", where)
				continue
			}
			id, _ := op["operationId"].(string)
			if id == "" {
				t.Errorf("%s has no operationId", where)
			} else if prev, dup := ids[id]; dup {
				t.Errorf("%s reuses operationId %s from %s", where, id, prev)
			}
			ids[id] = where
			if s, _ := op["summary"].(string); strings.TrimSpace(s) == "" {
				t.Errorf("%s has no summary", where)
			}
			tags, _ := op["tags"].([]any)
			if len(tags) == 0 {
				t.Errorf("%s has no tags", where)
			}
			for _, tag := range tags {
				if !declared[tag.(string)] {
					t.Errorf("%s uses undeclared tag %v", where, tag)
				}
			}
			responses, _ := op["responses"].(map[string]any)
			success := false
			for code := range responses {
				success = success || strings.HasPrefix(code, "2")
			}
			if !success {
				t.Errorf("%s documents no success response", where)
			}
			if security, has := op["security"].([]any); !has || len(security) > 0 {
				if public := map[string]bool{"/healthz": true, "/readyz": true, "/school/branding": true, "/auth/login": true, "/auth/refresh": true, "/auth/logout": true}; !public[path] {
					for _, code := range []string{"401", "403"} {
						if _, ok := responses[code]; !ok {
							t.Errorf("%s is secured but does not document %s", where, code)
						}
					}
				}
			}
			names := map[string]bool{}
			for _, list := range []any{op["parameters"], item["parameters"]} {
				params, _ := list.([]any)
				for _, p := range params {
					m, _ := p.(map[string]any)
					if ref, ok := m["$ref"].(string); ok {
						m = spec.Components.Parameters[strings.TrimPrefix(ref, "#/components/parameters/")]
					}
					if m["in"] == "path" {
						names[m["name"].(string)] = true
					}
				}
			}
			for _, match := range pathParam.FindAllStringSubmatch(path, -1) {
				if !names[match[1]] {
					t.Errorf("%s does not declare path parameter %s", where, match[1])
				}
			}
		}
	}
	if len(ids) < 240 {
		t.Fatalf("only %d operations found", len(ids))
	}
}

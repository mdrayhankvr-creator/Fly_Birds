package httpapi

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

var ginParameter = regexp.MustCompile(`:([A-Za-z][A-Za-z0-9]*)`)

func TestOpenAPICoversRegisteredRoutes(t *testing.T) {
	body, err := os.ReadFile("../../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := yaml.Unmarshal(body, &spec); err != nil {
		t.Fatal(err)
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI paths missing")
	}
	components, ok := spec["components"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI components missing")
	}
	for _, route := range NewRouter().Routes() {
		path := ginParameter.ReplaceAllString(route.Path, "{$1}")
		definition, ok := paths[path].(map[string]any)
		if !ok {
			t.Errorf("OpenAPI missing path %s", path)
			continue
		}
		if _, ok := definition[strings.ToLower(route.Method)]; !ok {
			t.Errorf("OpenAPI missing %s %s", route.Method, path)
		}
	}
	var inspect func(any)
	inspect = func(value any) {
		switch entry := value.(type) {
		case map[string]any:
			if ref, ok := entry["$ref"].(string); ok {
				if !strings.HasPrefix(ref, "#/components/") {
					t.Errorf("unexpected reference %s", ref)
				} else {
					parts := strings.Split(strings.TrimPrefix(ref, "#/components/"), "/")
					if len(parts) != 2 {
						t.Errorf("invalid reference %s", ref)
					} else if group, ok := components[parts[0]].(map[string]any); !ok || group[parts[1]] == nil {
						t.Errorf("unresolved reference %s", ref)
					}
				}
			}
			for _, nested := range entry {
				inspect(nested)
			}
		case []any:
			for _, nested := range entry {
				inspect(nested)
			}
		}
	}
	inspect(spec)
}

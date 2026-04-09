package workflow

import "testing"

func TestParseFrontMatterAndPrompt(t *testing.T) {
	loaded, err := Parse([]byte(`---
tracker:
  kind: fizzy
---

Hello {{ issue.identifier }}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if loaded.Config["tracker"] == nil {
		t.Fatalf("expected tracker config")
	}
	if loaded.Prompt != "Hello {{ issue.identifier }}" {
		t.Fatalf("unexpected prompt: %q", loaded.Prompt)
	}
}

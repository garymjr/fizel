package workflow

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Loaded struct {
	Config         map[string]any
	Prompt         string
	PromptTemplate string
	Hash           [32]byte
}

func Load(path string) (Loaded, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Loaded{}, fmt.Errorf("read workflow: %w", err)
	}
	return Parse(content)
}

func Parse(content []byte) (Loaded, error) {
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	frontMatter, promptLines := splitFrontMatter(lines)

	cfg := map[string]any{}
	if strings.TrimSpace(strings.Join(frontMatter, "\n")) != "" {
		if err := yaml.Unmarshal([]byte(strings.Join(frontMatter, "\n")), &cfg); err != nil {
			return Loaded{}, fmt.Errorf("parse workflow front matter: %w", err)
		}
		if cfg == nil {
			return Loaded{}, errors.New("workflow front matter must decode to a map")
		}
	}

	prompt := strings.TrimSpace(strings.Join(promptLines, "\n"))
	return Loaded{
		Config:         cfg,
		Prompt:         prompt,
		PromptTemplate: prompt,
		Hash:           sha256.Sum256(content),
	}, nil
}

func splitFrontMatter(lines []string) ([]string, []string) {
	if len(lines) == 0 || lines[0] != "---" {
		return nil, lines
	}
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			return lines[1:i], lines[i+1:]
		}
	}
	return lines[1:], nil
}

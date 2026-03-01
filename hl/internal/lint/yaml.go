package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iamkhattar/homelab/hl/internal/ui"
	"gopkg.in/yaml.v3"
)

// skipDirs are directories to exclude from YAML validation.
var skipDirs = map[string]bool{
	".git":         true,
	".idea":        true,
	"node_modules": true,
	"vendor":       true,
	"charts":       true,     // Helm dependency charts
	"templates":    true,     // Helm templates contain Go template syntax
}

// YAMLLinter validates YAML syntax across the entire repo using the native
// Go yaml.v3 parser — no external tools required.
type YAMLLinter struct{}

func (y *YAMLLinter) Name() string { return "yaml" }
func (y *YAMLLinter) CanFix() bool { return false }

func (y *YAMLLinter) Fix(_ string) error { return nil }

func (y *YAMLLinter) Lint(root string) error {
	var invalid []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		// Validate by unmarshalling into a generic interface.
		var doc interface{}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			rel, _ := filepath.Rel(root, path)
			ui.KeyValue("invalid", fmt.Sprintf("%s: %s", rel, err))
			invalid = append(invalid, rel)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if len(invalid) > 0 {
		return fmt.Errorf("%d YAML file(s) have syntax errors", len(invalid))
	}
	return nil
}

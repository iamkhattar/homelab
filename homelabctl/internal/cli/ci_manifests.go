package cli

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// apiGroupsFile is the allowlist of API groups the cluster charts may use.
const apiGroupsFile = "cluster/api-groups.txt"

// checkClusterAPIGroups verifies that every apiVersion used by the cluster
// charts names a group the platform actually installs.
//
// Helm renders templates without validating them against a cluster, so a
// mistyped group - secrets.hashiicorp.com instead of secrets.hashicorp.com -
// renders cleanly and is only discovered when helmfile diff runs against a live
// cluster, halfway through a deployment. This check catches that in CI.
func checkClusterAPIGroups(_ *cobra.Command, s *state) error {
	allowed, err := readAPIGroupAllowlist(filepath.Join(s.root, apiGroupsFile))
	if err != nil {
		return err
	}

	unknown := map[string][]string{}
	clusterDir := filepath.Join(s.root, "cluster")
	err = filepath.WalkDir(clusterDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			// Vendored charts carry their own manifests and are checked
			// upstream; only first-party templates are in scope.
			if entry.Name() == "charts" {
				return fs.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".yaml", ".yml":
		default:
			return nil
		}
		relative, err := filepath.Rel(s.root, path)
		if err != nil {
			relative = path
		}
		return scanAPIGroups(path, filepath.ToSlash(relative), allowed, unknown)
	})
	if err != nil {
		return err
	}
	if len(unknown) == 0 {
		return nil
	}

	groups := make([]string, 0, len(unknown))
	for group := range unknown {
		groups = append(groups, group)
	}
	sort.Strings(groups)

	var message strings.Builder
	message.WriteString("unknown API groups in cluster manifests (add the group to " + apiGroupsFile + " only if it is correct):")
	for _, group := range groups {
		locations := unknown[group]
		sort.Strings(locations)
		if len(locations) > 3 {
			locations = locations[:3]
		}
		message.WriteString("\n  " + group + " at " + strings.Join(locations, ", "))
	}
	return fmt.Errorf("%s", message.String())
}

// scanAPIGroups records every apiVersion group in one manifest file that the
// allowlist does not contain.
func scanAPIGroups(path, relative string, allowed map[string]bool, unknown map[string][]string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		value, ok := apiVersionValue(scanner.Text())
		if !ok {
			continue
		}
		group := apiVersionGroup(value)
		if group == "" || allowed[group] {
			continue
		}
		unknown[group] = append(unknown[group], fmt.Sprintf("%s:%d", relative, line))
	}
	return scanner.Err()
}

// apiVersionValue extracts the value of an apiVersion line. Templated values
// are ignored: they are resolved at render time.
func apiVersionValue(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	const prefix = "apiVersion:"
	if !strings.HasPrefix(trimmed, prefix) {
		return "", false
	}
	value := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
	value = strings.Trim(value, `"'`)
	if value == "" || strings.Contains(value, "{{") {
		return "", false
	}
	return value, true
}

// apiVersionGroup returns the group of an apiVersion. The core group is empty.
func apiVersionGroup(version string) string {
	if index := strings.Index(version, "/"); index >= 0 {
		return version[:index]
	}
	return ""
}

// readAPIGroupAllowlist reads the allowed groups, ignoring comments and blanks.
func readAPIGroupAllowlist(path string) (map[string]bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read API group allowlist: %w", err)
	}
	defer file.Close()

	allowed := map[string]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		allowed[line] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read API group allowlist: %w", err)
	}
	return allowed, nil
}

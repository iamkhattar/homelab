package inventory

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Connection contains the Ansible connection variables needed by native SSH
// commands. It deliberately excludes configuration-management variables.
type Connection struct {
	Address      string
	User         string
	Port         int
	IdentityFile string
}

type group struct {
	Hosts    map[string]map[string]any `yaml:"hosts"`
	Children map[string]group          `yaml:"children"`
	Vars     map[string]any            `yaml:"vars"`
}

// ResolveConnection reads an Ansible YAML inventory without invoking Ansible.
// The supported shape is the repository's declarative group/children/hosts
// structure. Parent group variables are inherited and host variables win.
func ResolveConnection(path, host string) (Connection, error) {
	// #nosec G304 -- path is the repository's fixed private inventory file.
	content, err := os.ReadFile(path)
	if err != nil {
		return Connection{}, fmt.Errorf("reading private inventory %s: %w", path, err)
	}

	groups := map[string]group{}
	if err := yaml.Unmarshal(content, &groups); err != nil {
		return Connection{}, fmt.Errorf("parsing private inventory %s: %w", path, err)
	}
	if len(groups) == 0 {
		return Connection{}, fmt.Errorf("private inventory %s is empty", path)
	}

	matches := make([]map[string]any, 0, 1)
	for _, name := range sortedGroupNames(groups) {
		findHost(groups[name], host, nil, &matches)
	}
	if len(matches) == 0 {
		return Connection{}, fmt.Errorf("inventory host %q was not found in %s", host, path)
	}
	variables, err := mergeConnectionMatches(host, matches)
	if err != nil {
		return Connection{}, err
	}
	return connectionFromVariables(host, variables)
}

func findHost(current group, host string, inherited map[string]any, matches *[]map[string]any) {
	variables := mergeVariables(inherited, current.Vars)
	if hostVariables, ok := current.Hosts[host]; ok {
		*matches = append(*matches, mergeVariables(variables, hostVariables))
	}
	for _, name := range sortedGroupNames(current.Children) {
		findHost(current.Children[name], host, variables, matches)
	}
}

func mergeVariables(base, override map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(override))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func mergeConnectionMatches(host string, matches []map[string]any) (map[string]any, error) {
	merged := map[string]any{}
	normalized := map[string]string{}
	for _, match := range matches {
		for _, key := range []string{"ansible_host", "ansible_user", "ansible_port", "ansible_ssh_private_key_file"} {
			value, ok := match[key]
			if !ok || value == nil {
				continue
			}
			comparable, err := comparableConnectionValue(key, value)
			if err != nil {
				return nil, err
			}
			if previous, exists := normalized[key]; exists && previous != comparable {
				return nil, fmt.Errorf("inventory host %q has conflicting %s values across groups", host, key)
			}
			normalized[key] = comparable
			merged[key] = value
		}
	}
	return merged, nil
}

func comparableConnectionValue(key string, value any) (string, error) {
	if key == "ansible_port" {
		port, err := parsePort(value)
		if err != nil {
			return "", fmt.Errorf("invalid ansible_port: %w", err)
		}
		return strconv.Itoa(port), nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("inventory variable %s must be a string", key)
	}
	return text, nil
}

func sortedGroupNames[T any](groups map[string]T) []string {
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func connectionFromVariables(host string, values map[string]any) (Connection, error) {
	connection := Connection{Address: host, Port: 22}
	var err error
	if connection.Address, err = optionalString(values, "ansible_host", connection.Address); err != nil {
		return Connection{}, err
	}
	if connection.User, err = optionalString(values, "ansible_user", ""); err != nil {
		return Connection{}, err
	}
	if connection.IdentityFile, err = optionalString(values, "ansible_ssh_private_key_file", ""); err != nil {
		return Connection{}, err
	}
	if value, ok := values["ansible_port"]; ok {
		connection.Port, err = parsePort(value)
		if err != nil {
			return Connection{}, fmt.Errorf("invalid ansible_port for %s: %w", host, err)
		}
	}
	if err := validateConnection(host, connection); err != nil {
		return Connection{}, err
	}
	return connection, nil
}

func optionalString(values map[string]any, key, fallback string) (string, error) {
	value, ok := values[key]
	if !ok || value == nil {
		return fallback, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("inventory variable %s must be a string", key)
	}
	if strings.TrimSpace(text) == "" {
		return fallback, nil
	}
	return text, nil
}

func parsePort(value any) (int, error) {
	var port int
	switch typed := value.(type) {
	case int:
		port = typed
	case string:
		parsed, err := strconv.Atoi(typed)
		if err != nil {
			return 0, fmt.Errorf("%q is not a number", typed)
		}
		port = parsed
	default:
		return 0, fmt.Errorf("must be a number or numeric string")
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("%d is outside 1-65535", port)
	}
	return port, nil
}

func validateConnection(host string, connection Connection) error {
	if connection.User == "change-me" {
		return fmt.Errorf("inventory host %q still uses the example ansible_user %q", host, connection.User)
	}
	for label, value := range map[string]string{
		"ansible_host": connection.Address,
		"ansible_user": connection.User,
	} {
		if strings.HasPrefix(value, "-") || strings.ContainsAny(value, " \t\r\n") {
			return fmt.Errorf("inventory variable %s contains an unsafe SSH value", label)
		}
	}
	if strings.Contains(connection.User, "@") {
		return fmt.Errorf("inventory variable ansible_user must not contain @")
	}
	return nil
}

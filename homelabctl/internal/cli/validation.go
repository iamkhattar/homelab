package cli

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	inventoryHostPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	releaseNamePattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)
	imageTagPattern       = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
	gitSHAImageTagPattern = regexp.MustCompile(`^[0-9a-f]{40,64}$`)
	apiIdentifierPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

func validateAPIIdentifier(value, label string) error {
	if !apiIdentifierPattern.MatchString(value) {
		return fmt.Errorf("%s must be 1-128 characters using letters, numbers, dots, underscores, colons, or hyphens", label)
	}
	return nil
}

func validateInventoryHost(value string) error {
	if !inventoryHostPattern.MatchString(value) {
		return fmt.Errorf("host must start with a letter or number and use only letters, numbers, dots, underscores, or hyphens")
	}
	return nil
}

func validateReleaseName(value string) error {
	if !releaseNamePattern.MatchString(value) {
		return fmt.Errorf("release must use lowercase letters, numbers, dots, or hyphens")
	}
	return nil
}

func validateTags(tags []string) error {
	for _, tag := range tags {
		if !imageTagPattern.MatchString(tag) {
			return fmt.Errorf("invalid image tag %q", tag)
		}
	}
	return nil
}

func validateNonBlank(value, label string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must not be empty", label)
	}
	if strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s contains unsupported control characters", label)
	}
	return nil
}

func validateContainerValue(value, label string) error {
	if err := validateNonBlank(value, label); err != nil {
		return err
	}
	if strings.IndexFunc(value, func(character rune) bool { return character == ' ' || character == '\t' }) >= 0 {
		return fmt.Errorf("%s must not contain whitespace", label)
	}
	return nil
}

func validateSnapshotName(value string) error {
	if len(value) > 64 || !snapshotNamePattern.MatchString(value) {
		return fmt.Errorf("snapshot name must be 1-64 characters using letters, numbers, dots, underscores, or hyphens")
	}
	return nil
}

func validateRecoveryDestination(root, destination string) (string, error) {
	return validateExternalDestination(root, destination, "recovery destination")
}

func validateExternalDestination(root, destination, label string) (string, error) {
	if err := validateNonBlank(destination, label); err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", label, err)
	}
	if filepath.Dir(absolute) == absolute {
		return "", fmt.Errorf("%s must not be a filesystem root", label)
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return "", fmt.Errorf("comparing %s with repository: %w", label, err)
	}
	if relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s must be outside the repository", label)
	}
	return absolute, nil
}

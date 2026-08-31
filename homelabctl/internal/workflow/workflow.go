package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

type definition struct {
	Name        string         `yaml:"name"`
	On          yaml.Node      `yaml:"on"`
	Permissions map[string]any `yaml:"permissions"`
	Concurrency yaml.Node      `yaml:"concurrency"`
	Env         map[string]any `yaml:"env"`
	Jobs        map[string]job `yaml:"jobs"`
}

type job struct {
	Needs          any            `yaml:"needs"`
	If             string         `yaml:"if"`
	TimeoutMinutes int            `yaml:"timeout-minutes"`
	Steps          []step         `yaml:"steps"`
	Permissions    map[string]any `yaml:"permissions"`
}

type step struct {
	Name string         `yaml:"name"`
	Uses string         `yaml:"uses"`
	Run  string         `yaml:"run"`
	If   string         `yaml:"if"`
	With map[string]any `yaml:"with"`
	Env  map[string]any `yaml:"env"`
}

// ValidateDirectory parses every workflow and enforces the repository's CI
// contract. GitHub remains the source of truth for the complete Actions schema;
// these checks protect the invariants homelabctl relies on.
func ValidateDirectory(root string) error {
	directory := filepath.Join(root, ".github", "workflows")
	paths, err := filepath.Glob(filepath.Join(directory, "*.y*ml"))
	if err != nil {
		return fmt.Errorf("finding GitHub workflows: %w", err)
	}
	if len(paths) == 0 {
		return fmt.Errorf("no GitHub workflows found in %s", directory)
	}
	sort.Strings(paths)

	var problems []error
	for _, path := range paths {
		// #nosec G304 -- every path comes from the fixed .github/workflows glob
		// under the already validated repository root.
		contents, err := os.ReadFile(path)
		if err != nil {
			problems = append(problems, fmt.Errorf("reading %s: %w", path, err))
			continue
		}
		var workflow definition
		if err := yaml.Unmarshal(contents, &workflow); err != nil {
			problems = append(problems, fmt.Errorf("parsing %s: %w", path, err))
			continue
		}
		if workflow.Name == "" || len(workflow.Jobs) == 0 {
			problems = append(problems, fmt.Errorf("%s must define a name and at least one job", path))
		}
		problems = append(problems, validateGeneral(path, workflow)...)
		if filepath.Base(path) == "ci.yml" || filepath.Base(path) == "ci.yaml" {
			problems = append(problems, validateCI(path, workflow)...)
		}
		if filepath.Base(path) == "auto-assign.yml" || filepath.Base(path) == "auto-assign.yaml" {
			problems = append(problems, validateAutoAssign(root, path, workflow)...)
		}
	}
	return errors.Join(problems...)
}

var majorVersionPattern = regexp.MustCompile(`@v([0-9]+)(?:\.|$)`)

const (
	pinnedAutoAssignAction = "kentaro-m/auto-assign-action@a6d59add3a817df08cafa9b166367768d2c337f8"
	pinnedGoReleaserAction = "goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94"
)

var minimumActionMajors = map[string]int{
	"actions/cache/restore":             6,
	"actions/cache/save":                6,
	"actions/checkout":                  7,
	"actions/upload-artifact":           7,
	"actions/setup-go":                  7,
	"actions/setup-node":                7,
	"actions/setup-python":              7,
	"docker/login-action":               4,
	"goreleaser/goreleaser-action":      7,
	"github/codeql-action/upload-sarif": 4,
	"hashicorp/setup-terraform":         4,
}

func validateGeneral(path string, workflow definition) []error {
	var problems []error
	if emptyNode(workflow.On) {
		problems = append(problems, fmt.Errorf("%s must define at least one trigger", path))
	}
	for jobName, candidate := range workflow.Jobs {
		if candidate.TimeoutMinutes <= 0 {
			problems = append(problems, fmt.Errorf("%s: job %s must define timeout-minutes", path, jobName))
		}
		if len(workflow.Permissions) == 0 && len(candidate.Permissions) == 0 {
			problems = append(problems, fmt.Errorf("%s: job %s must use explicit permissions", path, jobName))
		}
		for _, candidateStep := range candidate.Steps {
			parts := strings.SplitN(candidateStep.Uses, "@", 2)
			if len(parts) != 2 {
				continue
			}
			minimum, guarded := minimumActionMajors[parts[0]]
			if !guarded {
				continue
			}
			matches := majorVersionPattern.FindStringSubmatch(candidateStep.Uses)
			if len(matches) != 2 {
				// Full commit hashes are intentionally allowed for immutable pinning.
				if len(parts[1]) == 40 {
					continue
				}
				problems = append(problems, fmt.Errorf("%s: job %s action %s must use v%d or newer, or a full commit hash", path, jobName, candidateStep.Uses, minimum))
				continue
			}
			major, _ := strconv.Atoi(matches[1])
			if major < minimum {
				problems = append(problems, fmt.Errorf("%s: job %s action %s is stale; use v%d or newer", path, jobName, candidateStep.Uses, minimum))
			}
		}
	}
	return append(problems, forbiddenCommands(path, workflow)...)
}

type autoAssignConfig struct {
	AddReviewers bool   `yaml:"addReviewers"`
	AddAssignees string `yaml:"addAssignees"`
	RunOnDraft   bool   `yaml:"runOnDraft"`
}

func validateAutoAssign(root, path string, workflow definition) []error {
	var problems []error
	problem := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf("%s: %s", path, fmt.Sprintf(format, args...)))
	}

	if !mappingKeys(workflow.On)["pull_request_target"] {
		problem("must use pull_request_target so fork pull requests receive write-capable assignment")
	}
	assign, ok := workflow.Jobs["assign-author"]
	if !ok {
		problem("must define the assign-author job")
	} else {
		if value(assign.Permissions["issues"]) != "write" {
			problem("assign-author job must grant issues: write")
		}
		action, found := findUses(assign.Steps, "kentaro-m/auto-assign-action@")
		if !found || value(action.With["configuration-path"]) != ".github/auto_assign.yml" {
			problem("assign-author job must use the reviewed auto-assign action and configuration")
		} else if action.Uses != pinnedAutoAssignAction {
			problem("assign-author job must pin the auto-assign action to the reviewed commit")
		}
		for _, candidate := range assign.Steps {
			if strings.HasPrefix(candidate.Uses, "actions/checkout@") {
				problem("assign-author job must not check out pull request code")
			}
		}
	}

	configPath := filepath.Join(root, ".github", "auto_assign.yml")
	// #nosec G304 -- the path is fixed beneath the validated repository root.
	contents, err := os.ReadFile(configPath)
	if err != nil {
		problem("reading assignment configuration: %v", err)
		return problems
	}
	var config autoAssignConfig
	if err := yaml.Unmarshal(contents, &config); err != nil {
		problem("parsing assignment configuration: %v", err)
		return problems
	}
	if config.AddReviewers {
		problem("must not request the pull request author as their own reviewer")
	}
	if config.AddAssignees != "author" {
		problem("must assign the pull request author explicitly")
	}
	if !config.RunOnDraft {
		problem("must assign draft pull requests when they are opened")
	}
	return problems
}

func validateCI(path string, workflow definition) []error {
	var problems []error
	problem := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf("%s: %s", path, fmt.Sprintf(format, args...)))
	}

	events := mappingKeys(workflow.On)
	if !events["push"] || !events["pull_request"] {
		problem("must run for pushes and pull requests")
	}
	if value(workflow.Permissions["contents"]) != "read" {
		problem("must default to contents: read permissions")
	}
	if value(workflow.Env["GO_VERSION"]) != "1.27.0" {
		problem("must use Go 1.27.0")
	}
	if value(workflow.Env["NODE_VERSION"]) != "24" {
		problem("must use Node 24")
	}
	if value(workflow.Env["RELEASE_VERSION"]) != "v0.1.${{ github.run_number }}" {
		problem("must define the shared immutable release version as v0.1.${{ github.run_number }}")
	}
	if emptyNode(workflow.Concurrency) {
		problem("must define concurrency so stale runs are cancelled")
	}

	check, ok := workflow.Jobs["check"]
	if !ok {
		problem("must define the check job")
	} else {
		validateJob(path, "check", check, &problems)
		if value(check.Permissions["security-events"]) != "write" {
			problem("check job must grant security-events: write for SARIF upload")
		}
		setupGo, found := findUses(check.Steps, "actions/setup-go@")
		if !found || value(setupGo.With["cache"]) != "false" {
			problem("check job must use the pre-check Go cache instead of setup-go post-job caching")
		}
		goRestore, goRestored := findUsesWithPath(check.Steps, "actions/cache/restore@", "~/go/pkg/mod")
		goSave, goSaved := findUsesWithPath(check.Steps, "actions/cache/save@", "~/go/pkg/mod")
		goCachePaths := value(goRestore.With["path"])
		goCacheKey := value(goRestore.With["key"])
		if !goRestored || !strings.Contains(goCachePaths, "~/.cache/go-build") || !strings.Contains(goCacheKey, "homelabctl/go.sum") || !strings.Contains(goCacheKey, "butler/go.sum") {
			problem("check job must restore Go modules and build output using every Go module checksum")
		}
		if !goSaved || !strings.Contains(goSave.If, "cache-hit != 'true'") || value(goSave.With["key"]) != "${{ steps.go-runtime-cache.outputs.cache-primary-key }}" {
			problem("check job must save a missing Go cache before repository checks")
		}
		trivyRestore, trivyRestored := findUsesWithPath(check.Steps, "actions/cache/restore@", "trivy-cache")
		trivySave, trivySaved := findUsesWithPath(check.Steps, "actions/cache/save@", "trivy-cache")
		trivyCacheKey := value(trivyRestore.With["key"])
		if !trivyRestored || !strings.Contains(trivyCacheKey, "0.74.0") || !strings.Contains(trivyCacheKey, "steps.trivy-cache-epoch.outputs.day") || !strings.Contains(value(trivyRestore.With["restore-keys"]), "trivy-${{ runner.os }}-${{ runner.arch }}-0.74.0-") {
			problem("check job must restore the daily Trivy database and checks cache")
		}
		if !trivySaved || !strings.Contains(trivySave.If, "always()") || !strings.Contains(trivySave.If, "cache-hit != 'true'") || value(trivySave.With["key"]) != "${{ steps.trivy-runtime-cache.outputs.cache-primary-key }}" {
			problem("check job must save the Trivy cache even when repository checks fail")
		}
		ansibleRestore, restored := findUsesWithPath(check.Steps, "actions/cache/restore@", "ansible/.venv")
		ansibleSave, saved := findUsesWithPath(check.Steps, "actions/cache/save@", "ansible/.venv")
		ansibleCachePaths := value(ansibleRestore.With["path"])
		ansibleCacheKey := value(ansibleRestore.With["key"])
		if !restored || !strings.Contains(ansibleCachePaths, "ansible/.venv") || !strings.Contains(ansibleCachePaths, "ansible/collections") || !strings.Contains(ansibleCacheKey, "ansible/requirements.txt") || !strings.Contains(ansibleCacheKey, "ansible/requirements.yml") {
			problem("check job must restore the pinned Ansible runtime using both requirements files")
		}
		if !saved || !strings.Contains(ansibleSave.If, "cache-hit != 'true'") || value(ansibleSave.With["key"]) != "${{ steps.ansible-runtime-cache.outputs.cache-primary-key }}" {
			problem("check job must save a missing Ansible runtime before repository checks")
		}
		if !hasRun(check.Steps, "bin/homelabctl setup ansible") || !hasRun(check.Steps, "bin/homelabctl setup docs") || !hasRun(check.Steps, "bin/homelabctl setup go") || !hasRun(check.Steps, "bin/homelabctl setup reports") {
			problem("check job must install Ansible, docs, Go and reporting dependencies through homelabctl")
		}
		if !hasRun(check.Steps, "bin/homelabctl ci check", "--reports") {
			problem("check job must generate reports through bin/homelabctl ci check --reports")
		}
		if hasRun(check.Steps, "bin/homelabctl ci check", "--skip", "go-test") {
			problem("check job must not skip Go tests")
		}
		artifactStep, found := findUses(check.Steps, "actions/upload-artifact@")
		if !found || !strings.Contains(artifactStep.If, "always()") || !strings.Contains(value(artifactStep.With["path"]), "test-results/") || !strings.Contains(value(artifactStep.With["path"]), "sarif/") || !strings.Contains(value(artifactStep.With["path"]), "sbom/") {
			problem("check job must upload test, SARIF and SBOM report directories")
		}
		type sarifUpload struct {
			path     string
			category string
		}
		requiredUploads := []sarifUpload{
			{path: "sarif/gosec-homelabctl.sarif", category: "gosec-homelabctl"},
			{path: "sarif/gosec-butler.sarif", category: "gosec-butler"},
			{path: "sarif/trivy.sarif", category: "trivy-repository"},
		}
		var sarifSteps []step
		for _, candidate := range check.Steps {
			if strings.HasPrefix(candidate.Uses, "github/codeql-action/upload-sarif@") {
				sarifSteps = append(sarifSteps, candidate)
			}
		}
		if len(sarifSteps) != len(requiredUploads) {
			problem("check job must upload each homelabctl-generated SARIF file separately")
		}
		for _, required := range requiredUploads {
			found := false
			for _, candidate := range sarifSteps {
				if value(candidate.With["sarif_file"]) == required.path && value(candidate.With["category"]) == required.category && strings.Contains(candidate.If, "always()") {
					found = true
					break
				}
			}
			if !found {
				problem("check job must upload %s with unique category %s", required.path, required.category)
			}
		}
	}

	publish, ok := workflow.Jobs["publish"]
	if !ok {
		problem("must define the publish job")
	} else {
		validateJob(path, "publish", publish, &problems)
		if !containsString(publish.Needs, "check") {
			problem("publish job must depend on check")
		}
		if !hasRun(publish.Steps, "ci build", "--changed", `origin/${{ github.base_ref }}`, `--tag "${{ github.sha }}"`) {
			problem("publish job must build PR changes from the base branch with the commit SHA tag")
		}
		if !hasRun(publish.Steps, "ci publish", `--tag "${{ env.RELEASE_VERSION }}"`, "--tag latest", `--tag "${{ github.sha }}"`) {
			problem("publish job must publish all release images with the shared release version, latest, and commit SHA tags")
		}
		for _, candidate := range publish.Steps {
			if strings.Contains(candidate.Run, "ci publish") && strings.Contains(candidate.Run, "--changed") {
				problem("main publication must not use --changed because Butler and homelabctl share every release version")
			}
		}
		if !hasRunWithEnv(publish.Steps, "ci publish", "CI", "true") {
			problem("image publication must set CI=true")
		}
	}

	release, ok := workflow.Jobs["release"]
	if !ok {
		problem("must define the homelabctl release job")
	} else {
		validateJob(path, "release", release, &problems)
		if !containsString(release.Needs, "check") {
			problem("release job must depend on check")
		}
		if !containsString(release.Needs, "publish") {
			problem("release job must wait for image publication")
		}
		if value(release.Permissions["contents"]) != "write" {
			problem("release job must grant contents: write")
		}
		if !strings.Contains(release.If, "github.event_name == 'push'") || !strings.Contains(release.If, "github.ref == 'refs/heads/main'") {
			problem("release job must run only for pushes to main")
		}
		if !hasRun(release.Steps,
			`git show-ref --verify --quiet "$tag_ref"`,
			`git rev-list -n 1 "$tag_ref"`,
			`git tag --annotate "$RELEASE_VERSION" "$GITHUB_SHA"`,
			`git push origin "$tag_ref"`,
		) {
			problem("release job must create or verify an immutable tag at the event commit")
		}
		releaseStep, found := findUses(release.Steps, "goreleaser/goreleaser-action@")
		if !found {
			problem("release job must use goreleaser/goreleaser-action")
		} else {
			if releaseStep.Uses != pinnedGoReleaserAction {
				problem("release job must pin goreleaser/goreleaser-action to the reviewed commit")
			}
			if value(releaseStep.With["version"]) != "v2.17.1" {
				problem("release job must pin GoReleaser v2.17.1")
			}
			if value(releaseStep.With["args"]) != "release --clean" || value(releaseStep.With["workdir"]) != "homelabctl" {
				problem("release job must run release --clean from homelabctl")
			}
			if value(releaseStep.Env["GITHUB_TOKEN"]) != "${{ secrets.GITHUB_TOKEN }}" {
				problem("release job must authenticate with GITHUB_TOKEN")
			}
			if value(releaseStep.Env["GORELEASER_CURRENT_TAG"]) != "${{ env.RELEASE_VERSION }}" {
				problem("release job must use the same shared release version as container publication")
			}
		}
	}
	return problems
}

func validateJob(path, name string, candidate job, problems *[]error) {
	if candidate.TimeoutMinutes <= 0 {
		*problems = append(*problems, fmt.Errorf("%s: %s job must define timeout-minutes", path, name))
	}
	foundCheckout := false
	for _, candidateStep := range candidate.Steps {
		if strings.HasPrefix(candidateStep.Uses, "actions/checkout@") && value(candidateStep.With["fetch-depth"]) != "0" {
			foundCheckout = true
			*problems = append(*problems, fmt.Errorf("%s: %s job checkout must use fetch-depth: 0 for merge-base calculation", path, name))
			return
		}
		if strings.HasPrefix(candidateStep.Uses, "actions/checkout@") {
			foundCheckout = true
		}
	}
	if !foundCheckout {
		*problems = append(*problems, fmt.Errorf("%s: %s job must check out the repository", path, name))
	}
}

func forbiddenCommands(path string, workflow definition) []error {
	forbidden := []string{"homelabctl deploy", "terraform apply", "terraform destroy", "helmfile apply", "helmfile sync", "kubectl apply"}
	var problems []error
	for jobName, candidate := range workflow.Jobs {
		for _, candidateStep := range candidate.Steps {
			normalized := strings.ToLower(strings.Join(strings.Fields(candidateStep.Run), " "))
			for _, command := range forbidden {
				if strings.Contains(normalized, command) {
					problems = append(problems, fmt.Errorf("%s: job %s contains forbidden mutating command %q", path, jobName, command))
				}
			}
		}
	}
	return problems
}

func mappingKeys(node yaml.Node) map[string]bool {
	result := map[string]bool{}
	if node.Kind == yaml.MappingNode {
		for index := 0; index < len(node.Content); index += 2 {
			result[node.Content[index].Value] = true
		}
	}
	return result
}

func emptyNode(node yaml.Node) bool { return node.Kind == 0 || len(node.Content) == 0 }

func hasRun(steps []step, fragments ...string) bool {
	for _, candidate := range steps {
		matches := true
		for _, fragment := range fragments {
			if !strings.Contains(candidate.Run, fragment) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func hasRunWithEnv(steps []step, runFragment, key, expected string) bool {
	for _, candidate := range steps {
		if strings.Contains(candidate.Run, runFragment) && value(candidate.Env[key]) == expected {
			return true
		}
	}
	return false
}

func findUses(steps []step, prefix string) (step, bool) {
	for _, candidate := range steps {
		if strings.HasPrefix(candidate.Uses, prefix) {
			return candidate, true
		}
	}
	return step{}, false
}

func findUsesWithPath(steps []step, prefix, pathFragment string) (step, bool) {
	for _, candidate := range steps {
		if strings.HasPrefix(candidate.Uses, prefix) && strings.Contains(value(candidate.With["path"]), pathFragment) {
			return candidate, true
		}
	}
	return step{}, false
}

func containsString(candidate any, expected string) bool {
	switch typed := candidate.(type) {
	case string:
		return typed == expected
	case []any:
		for _, item := range typed {
			if value(item) == expected {
				return true
			}
		}
	}
	return false
}

func value(candidate any) string { return fmt.Sprint(candidate) }

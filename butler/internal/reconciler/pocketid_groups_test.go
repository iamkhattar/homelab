package reconciler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/iamkhattar/homelab/butler/internal/platform"
	"github.com/iamkhattar/homelab/butler/internal/pocketid"
)

// groupFixture is a Pocket ID user-group API stub.
type groupFixture struct {
	log     *callLog
	groups  map[string]pocketid.UserGroup
	created int
	updated int
}

func newGroupServer(t *testing.T, log *callLog, groups ...pocketid.UserGroup) *httptest.Server {
	t.Helper()
	fixture := &groupFixture{log: log, groups: map[string]pocketid.UserGroup{}}
	for _, g := range groups {
		fixture.groups[g.Name] = g
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/user-groups":
			fixture.log.add(http.MethodGet, "/api/user-groups", "")
			var data []pocketid.UserGroup
			for _, g := range fixture.groups {
				data = append(data, g)
			}
			writeJSON(w, map[string]any{"data": data})
		case r.Method == http.MethodPost && r.URL.Path == "/api/user-groups":
			fixture.created++
			var in pocketid.UserGroup
			_ = json.Unmarshal(raw, &in)
			fixture.log.add(http.MethodPost, "/api/user-groups", string(raw))
			in.ID = "group-" + in.Name
			fixture.groups[in.Name] = in
			writeJSON(w, in)
		case r.Method == http.MethodPut && len(r.URL.Path) > len("/api/user-groups/"):
			fixture.updated++
			id := r.URL.Path[len("/api/user-groups/"):]
			fixture.log.add(http.MethodPut, "/api/user-groups/"+id, string(raw))
			var in pocketid.UserGroup
			_ = json.Unmarshal(raw, &in)
			for name, g := range fixture.groups {
				if g.ID == id {
					in.ID = id
					fixture.groups[name] = in
				}
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func newGroup(name, friendly string) *unstructured.Unstructured {
	return unstructuredGroup(name, friendly)
}

func TestPocketIDGroupsCreatesMissingGroup(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	server := newGroupServer(t, log)
	vc := newVaultServer(t, func(method, path string, _ []byte) (int, []byte) {
		if method == http.MethodGet && path == "security/pocket-id" {
			return http.StatusOK, kvSecret(map[string]any{"static-api-key": "machine-key"})
		}
		return http.StatusNotFound, nil
	}, log)

	store := newPlatformStore(newGroup("butleradmin", "Homelab Admin"))
	r := NewPocketIDGroups(vc, store, server.URL)
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if got := log.mutations(); len(got) != 1 || got[0].Method != http.MethodPost || got[0].Path != "/api/user-groups" {
		t.Fatalf("expected exactly one group creation, got:%v", log)
	}
	status := statusOf(t, store, "PocketIDGroup")["butleradmin"]
	if reason := conditionReason(status); reason != "Reconciled" {
		t.Fatalf("expected Reconciled, got %q", reason)
	}
	if status.ProviderID != "group-butleradmin" {
		t.Fatalf("expected the provider ID in status, got %q", status.ProviderID)
	}
}

func TestPocketIDGroupsUpdatesChangedFriendlyName(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	server := newGroupServer(t, log, pocketid.UserGroup{ID: "gid-1", Name: "butleradmin", FriendlyName: "Old Name"})
	vc := newVaultServer(t, func(method, path string, _ []byte) (int, []byte) {
		if method == http.MethodGet && path == "security/pocket-id" {
			return http.StatusOK, kvSecret(map[string]any{"static-api-key": "machine-key"})
		}
		return http.StatusNotFound, nil
	}, log)

	store := newPlatformStore(newGroup("butleradmin", "New Name"))
	r := NewPocketIDGroups(vc, store, server.URL)
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if log.indexOf(http.MethodPut, "/api/user-groups/gid-1") < 0 {
		t.Fatalf("expected the renamed group to be updated, got:%v", log)
	}
	if log.indexOf(http.MethodPost, "/api/user-groups") >= 0 {
		t.Fatalf("an existing group must not be recreated, got:%v", log)
	}
}

func TestPocketIDGroupsIsIdempotent(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	server := newGroupServer(t, log, pocketid.UserGroup{ID: "gid-1", Name: "butleradmin", FriendlyName: "Homelab Admin"})
	vc := newVaultServer(t, func(method, path string, _ []byte) (int, []byte) {
		if method == http.MethodGet && path == "security/pocket-id" {
			return http.StatusOK, kvSecret(map[string]any{"static-api-key": "machine-key"})
		}
		return http.StatusNotFound, nil
	}, log)

	r := NewPocketIDGroups(vc, newPlatformStore(newGroup("butleradmin", "Homelab Admin")), server.URL)
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := log.mutations(); len(got) != 0 {
		t.Fatalf("an unchanged group must cause no mutation, got:%v", log)
	}
}

func TestPocketIDGroupsRequiresFriendlyName(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	server := newGroupServer(t, log)
	vc := newVaultServer(t, func(method, path string, _ []byte) (int, []byte) {
		if method == http.MethodGet && path == "security/pocket-id" {
			return http.StatusOK, kvSecret(map[string]any{"static-api-key": "machine-key"})
		}
		return http.StatusNotFound, nil
	}, log)

	store := newPlatformStore(newGroup("broken", ""))
	r := NewPocketIDGroups(vc, store, server.URL)
	if err := r.Reconcile(context.Background()); err == nil {
		t.Fatal("expected an error for a group without friendlyName")
	}
	if got := log.mutations(); len(got) != 0 {
		t.Fatalf("invalid spec must not mutate the provider, got:%v", log)
	}
	status := statusOf(t, store, "PocketIDGroup")["broken"]
	if reason := conditionReason(status); reason != "ReconcileFailed" {
		t.Fatalf("expected ReconcileFailed, got %q", reason)
	}
}

func TestPocketIDGroupsWaitsForMachineCredential(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	server := newGroupServer(t, log)
	vc := newVaultServer(t, func(string, string, []byte) (int, []byte) {
		return http.StatusNotFound, nil
	}, log)

	store := newPlatformStore(newGroup("butleradmin", "Homelab Admin"))
	r := NewPocketIDGroups(vc, store, server.URL)
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("a missing machine credential must not fail the run: %v", err)
	}
	if log.indexOf(http.MethodGet, "/api/user-groups") >= 0 {
		t.Fatalf("the provider must not be called without a machine credential, got:%v", log)
	}
	if reason := conditionReason(statusOf(t, store, "PocketIDGroup")["butleradmin"]); reason != "AwaitingAPIKey" {
		t.Fatalf("expected AwaitingAPIKey, got %q", reason)
	}
}

func TestPocketIDGroupsSkipsProviderWithoutItemsOrURL(t *testing.T) {
	t.Parallel()
	for name, store := range map[string]*platform.Store{
		"no items": newPlatformStore(),
		"no url":   newPlatformStore(newGroup("butleradmin", "Homelab Admin")),
	} {
		t.Run(name, func(t *testing.T) {
			log := &callLog{}
			vc := newVaultServer(t, func(string, string, []byte) (int, []byte) {
				return http.StatusNotFound, nil
			}, log)
			url := "http://127.0.0.1:1"
			if name == "no url" {
				url = ""
			}
			if err := NewPocketIDGroups(vc, store, url).Reconcile(context.Background()); err != nil {
				t.Fatalf("Reconcile: %v", err)
			}
			if got := log.snapshot(); len(got) != 0 {
				t.Fatalf("expected no Vault or provider calls, got:%v", log)
			}
		})
	}
}

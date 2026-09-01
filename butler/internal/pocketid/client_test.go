package pocketid

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListClientsUsesPaginatedV2Response(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/oidc/clients" || r.URL.Query().Get("pagination[limit]") != "100" {
			t.Fatalf("unexpected request URL: %s", r.URL.String())
		}
		if got := r.Header.Get("X-API-KEY"); got != "test-key" {
			t.Fatalf("X-API-KEY = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{
				"id": "grafana-id", "name": "grafana", "callbackURLs": []string{"https://grafana.example/login"},
			}},
			"pagination": map[string]any{"totalPages": 1},
		})
	}))
	defer server.Close()

	clients, err := NewClient(server.URL, "test-key").ListClients(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(clients) != 1 || clients[0].ID != "grafana-id" || len(clients[0].CallbackURLs) != 1 {
		t.Fatalf("unexpected clients: %#v", clients)
	}
}

func TestCreateConfidentialClientCreatesOneTimeSecret(t *testing.T) {
	t.Parallel()
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/oidc/clients":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "vault/client", "name": "vault"})
		case "/api/oidc/clients/vault%2Fclient/secrets", "/api/oidc/clients/vault/client/secrets":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "secret-id", "secret": "generated-secret"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	created, err := NewClient(server.URL, "test-key").CreateClient(context.Background(), OIDCClient{
		Name: "vault", CallbackURLs: []string{"https://vault.example/callback"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Secret != "generated-secret" || created.SecretID != "secret-id" {
		t.Fatalf("created secret = %#v", created)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestClientSecretLifecycleUsesPocketIDV214Contract(t *testing.T) {
	t.Parallel()
	var deleted string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/oidc/clients/grafana/secrets":
			_ = json.NewEncoder(w).Encode([]OIDCClientSecret{{ID: "old", IsActive: true}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/oidc/clients/grafana/secrets":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(OIDCClientSecret{ID: "new", Secret: "one-time-value", IsActive: true})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/oidc/clients/grafana/secrets/old":
			deleted = "old"
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	secrets, err := client.ListClientSecrets(context.Background(), "grafana")
	if err != nil || len(secrets) != 1 || secrets[0].ID != "old" {
		t.Fatalf("secrets=%#v err=%v", secrets, err)
	}
	created, err := client.CreateSecret(context.Background(), "grafana")
	if err != nil || created.ID != "new" || created.Secret != "one-time-value" {
		t.Fatalf("created=%#v err=%v", created, err)
	}
	if err := client.DeleteClientSecret(context.Background(), "grafana", "old"); err != nil {
		t.Fatal(err)
	}
	if deleted != "old" {
		t.Fatalf("deleted = %q", deleted)
	}
}

func TestUserGroupLifecycleUsesV2Endpoints(t *testing.T) {
	t.Parallel()
	var updated UserGroup
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/user-groups":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []UserGroup{{ID: "g1", Name: "homelab-admin", FriendlyName: "Old"}}})
		case r.Method == http.MethodPut && r.URL.Path == "/api/user-groups/g1":
			if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(updated)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	groups, err := client.ListUserGroups(context.Background())
	if err != nil || len(groups) != 1 {
		t.Fatalf("groups=%#v err=%v", groups, err)
	}
	if err := client.UpdateUserGroup(context.Background(), groups[0].ID, UserGroup{Name: groups[0].Name, FriendlyName: "Administrators"}); err != nil {
		t.Fatal(err)
	}
	if updated.FriendlyName != "Administrators" {
		t.Fatalf("updated = %#v", updated)
	}
}

func TestUserLifecycleUsesPocketIDV2Contract(t *testing.T) {
	t.Parallel()
	var groups []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/users":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []User{{ID: "u1", Username: "sam"}}})
		case r.Method == http.MethodPut && r.URL.Path == "/api/users/u1/user-groups":
			var body struct {
				UserGroupIDs []string `json:"userGroupIds"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			groups = body.UserGroupIDs
			_ = json.NewEncoder(w).Encode(User{ID: "u1", Username: "sam"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	users, err := client.ListUsers(context.Background())
	if err != nil || len(users) != 1 || users[0].ID != "u1" {
		t.Fatalf("users=%#v err=%v", users, err)
	}
	if _, err := client.UpdateUserGroups(context.Background(), "u1", []string{"g1", "g2"}); err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[1] != "g2" {
		t.Fatalf("groups = %#v", groups)
	}
}

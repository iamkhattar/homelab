package identity

import (
	"context"
	"strings"
	"testing"

	"github.com/iamkhattar/homelab/butler/internal/pocketid"
)

type fakeSecrets map[string]interface{}

func (f fakeSecrets) ReadSecret(context.Context, string) (map[string]interface{}, error) {
	return f, nil
}

func (f fakeSecrets) WriteSecret(context.Context, string, map[string]interface{}) error { return nil }

func TestCreateUserRejectsAdminPromotion(t *testing.T) {
	service := NewService(fakeSecrets{"api-key": "unused"}, "http://127.0.0.1")
	_, err := service.CreateUser(context.Background(), pocketid.User{Username: "owner", IsAdmin: true})
	if err == nil || !strings.Contains(err.Error(), "does not create") {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateUserRequiresUsername(t *testing.T) {
	service := NewService(fakeSecrets{"api-key": "unused"}, "http://127.0.0.1")
	_, err := service.CreateUser(context.Background(), pocketid.User{})
	if err == nil || !strings.Contains(err.Error(), "username") {
		t.Fatalf("error = %v", err)
	}
}

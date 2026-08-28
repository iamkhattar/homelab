// Package access owns Butler's provider-independent authorization model.
package access

import "context"

type Role string

const (
	Viewer   Role = "viewer"
	Operator Role = "operator"
	Admin    Role = "admin"
)

type Principal struct {
	Subject string   `json:"subject"`
	Email   string   `json:"email,omitempty"`
	Groups  []string `json:"groups"`
	Role    Role     `json:"role"`
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(Principal)
	return principal, ok
}

func FromClaims(subject, email string, groups []string) Principal {
	role := Viewer
	for _, group := range groups {
		switch group {
		case "homelab-admin":
			role = Admin
		case "homelab-operator":
			if role != Admin {
				role = Operator
			}
		}
	}
	return Principal{Subject: subject, Email: email, Groups: groups, Role: role}
}

func Allows(actual, required Role) bool {
	rank := map[Role]int{Viewer: 1, Operator: 2, Admin: 3}
	return rank[actual] >= rank[required]
}

package targetapi

import (
	"errors"
	"net/http"
)

type Permission string

const (
	PermissionMatchTicketsRead     Permission = "match_tickets.read"
	PermissionMatchTicketsWrite    Permission = "match_tickets.write"
	PermissionBackfillTicketsRead  Permission = "backfill_tickets.read"
	PermissionBackfillTicketsWrite Permission = "backfill_tickets.write"
	PermissionPoliciesRead         Permission = "policies.read"
	PermissionPoliciesWrite        Permission = "policies.write"
)

type Principal struct {
	Subject     string
	Tenant      string
	Permissions map[Permission]bool
}

func (principal Principal) Allows(permission Permission) bool {
	return principal.Permissions[permission]
}

// Authenticator is implemented by a deployment-owned identity adapter. The
// target API never accepts a tenant from a path, query, or request body.
type Authenticator interface {
	Authenticate(*http.Request) (Principal, error)
}

type AuthenticatorFunc func(*http.Request) (Principal, error)

func (authenticate AuthenticatorFunc) Authenticate(request *http.Request) (Principal, error) {
	return authenticate(request)
}

// ErrUnauthenticated tells the transport that credentials are absent or
// invalid. Other authenticator errors are treated as provider unavailability.
var ErrUnauthenticated = errors.New("unauthenticated")

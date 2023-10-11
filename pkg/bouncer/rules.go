package bouncer

import (
	"net/http"
)

type Bouncer interface {
	Check(r *http.Request, rules ...Rule) (*Claims, error)
	Config() Config
}

// The RuleFunc type is an adapter to allow the use of
// ordinary functions as Rule
type RuleFunc func(Claims) bool

func (r RuleFunc) IsAuthorized(claims Claims) bool {
	return r(claims)
}

type Rule interface {
	IsAuthorized(Claims) bool
}

type Admin struct {
	Authorized bool
}

func (a Admin) IsAuthorized(claims Claims) bool {
	return a.Authorized && claims.Authorized
}

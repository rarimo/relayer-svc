package bouncer

import (
	"github.com/golang-jwt/jwt/v5"
	"gitlab.com/distributed_lab/ape"
	"gitlab.com/distributed_lab/ape/problems"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"net/http"
	"strings"
)

var parser = jwt.Parser{}

// ParseClaims - parses claims without signature verification and token validation.
// Returns ErrNotAllowed if token is malformed or not present
func ParseClaims(r *http.Request) (*Claims, error) {
	rawClaims := r.Header.Get("Authorization")
	if rawClaims == "" {
		return nil, ErrNotAllowed
	}

	rawClaims = strings.TrimPrefix(rawClaims, "Bearer ")

	var claims Claims
	_, _, err := parser.ParseUnverified(rawClaims, &claims)
	if err != nil {
		return nil, ErrNotAllowed
	}

	return &claims, nil
}

func RequestMiddleware(log *logan.Entry, bouncer Bouncer, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := bouncer.Check(r, Admin{Authorized: true})
		if err != nil {
			log.WithError(err).Debug("failed to parse JWT claims")
		}

		if errors.Cause(err) == ErrNotAllowed {
			ape.Render(w, problems.Unauthorized())
			return
		}

		if errors.Cause(err) == ErrForbidden {
			ape.Render(w, problems.Forbidden())
			return
		}

		next(w, r)
	}
}

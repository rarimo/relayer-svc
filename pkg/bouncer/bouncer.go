package bouncer

import (
	"gitlab.com/distributed_lab/logan/v3/errors"
	"net/http"
	"time"
)

var (
	//ErrForbidden - requester does not have sufficient permission to perform request -
	// non of the rules have not returned non nil response
	ErrForbidden = errors.New("forbidden")
	//ErrNotAllowed - token is malformed, expired or not present
	ErrNotAllowed = errors.New("not allowed")
)

type bouncer struct {
	cfg Config
}

type Config struct {
	// SkipChecks make any request with valid or missing token pass
	SkipChecks bool
	TTL        time.Duration
}

func New(opts Config) Bouncer {
	return &bouncer{opts}
}

// Check - checks against specified rules. Panics if no rules specified (to get claims without rules use ParseClaims)
// Returns: ErrNotAllowed -
// Returns ErrForbidden
func (c bouncer) Check(r *http.Request, rules ...Rule) (*Claims, error) {
	if len(rules) == 0 {
		panic(errors.New("at least one rule must be specified"))
	}
	claims, err := ParseClaims(r)
	if err != nil {
		if errors.Cause(err) == ErrNotAllowed && c.cfg.SkipChecks {
			return nil, nil
		}

		return nil, errors.Wrap(err, "failed to parse jwt")
	}

	if c.cfg.SkipChecks {
		return claims, nil
	}

	if claims == nil {
		return nil, ErrNotAllowed
	}

	for _, rule := range rules {
		if rule.IsAuthorized(*claims) {
			return claims, nil
		}
	}

	return nil, ErrForbidden
}

func (c bouncer) Config() Config {
	return c.cfg
}

package bouncer

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/rarimo/relayer-svc/pkg/secret"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"time"
)

type Claims struct {
	Authorized bool `json:"authorized"`
	jwt.RegisteredClaims
}

func GenerateJWT(cfg Config, vault secret.Vault, log *logan.Entry) {
	claims := Claims{
		Authorized: true,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(cfg.TTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)

	tokenString, err := token.SignedString(vault.Secret().Bouncer())
	if err != nil {
		panic(errors.Wrap(err, "failed to sign token"))
	}

	log.WithField("token", tokenString).Info("generated token")
}

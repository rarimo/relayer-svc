package secret

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/olegfomenko/solana-go"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

var (
	ErrUninitializedPrivateKey = errors.New("private key should be initialized")
	EvmSecretsNotInitialized   = errors.New("evm secrets should be initialized")
)

type Secret struct {
	evm     *EVMSecrets
	near    *NearSecrets
	solana  *SolanaSecrets
	rarimo  *RarimoSecrets
	bouncer *ecdsa.PrivateKey
}

func newSecret(evm *EVMSecrets, near *NearSecrets, solana solana.PrivateKey, rarimo *RarimoSecrets, bouncer *ecdsa.PrivateKey) (*Secret, error) {
	if evm == nil {
		return nil, errors.Wrap(EvmSecretsNotInitialized, "evm secrets are empty")
	}

	for _, v := range *evm {
		if v == nil {
			return nil, errors.Wrap(ErrUninitializedPrivateKey, fmt.Sprintf("evm private key is empty for the chain: %s", v))
		}
	}

	if bouncer == nil {
		panic(errors.Wrap(ErrUninitializedPrivateKey, "bouncer private key is empty"))
	}

	return &Secret{
		evm,
		near,
		NewSolanaSecrets(solana),
		rarimo,
		bouncer,
	}, nil
}

func (s *Secret) EVM() *EVMSecrets {
	return s.evm
}

func (s *Secret) Near() *NearSecrets {
	return s.near
}

func (s *Secret) Solana() *SolanaSecrets {
	return s.solana
}

func (s *Secret) Rarimo() *RarimoSecrets {
	return s.rarimo
}

func (s *Secret) Bouncer() *ecdsa.PrivateKey {
	return s.bouncer
}

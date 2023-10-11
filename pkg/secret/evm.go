package secret

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type EVMSecrets map[string]*ecdsa.PrivateKey

func evmSecretsFromMap(data map[string]interface{}) (*EVMSecrets, error) {
	result := make(EVMSecrets)
	for k, v := range data {
		strValue, ok := v.(string)
		if !ok {
			return nil, errors.Errorf("invalid private key for the chain: %s", k)
		}

		pk, err := crypto.HexToECDSA(strValue)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("failed to parse private key for the chain: %s", k))
		}

		result[k] = pk
	}

	return &result, nil
}

func (e *EVMSecrets) PublicKey(chain string) common.Address {
	if (*e)[chain] == nil {
		panic(errors.Wrap(ErrUninitializedPrivateKey, fmt.Sprintf("evm private key is empty for the chain: %s", chain)))
	}
	return crypto.PubkeyToAddress((*e)[chain].PublicKey)
}

func (e *EVMSecrets) PrivateKey(chain string) *ecdsa.PrivateKey {
	if (*e)[chain] == nil {
		panic(errors.Wrap(ErrUninitializedPrivateKey, fmt.Sprintf("evm private key is empty for the chain: %s", chain)))
	}
	return (*e)[chain]
}

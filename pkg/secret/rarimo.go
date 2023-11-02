package secret

import (
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type RarimoSecrets struct {
	key     *secp256k1.PrivKey
	address string
}

func rarimoSecretsFromMap(data string) (*RarimoSecrets, error) {
	result := RarimoSecrets{}
	var err error
	
	result.key = &secp256k1.PrivKey{Key: hexutil.MustDecode(data)}
	result.address, err = bech32.ConvertAndEncode("rarimo", result.key.PubKey().Address().Bytes())
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert rarimo address")
	}
	return &result, nil
}

func (s *RarimoSecrets) PublicKey() string {
	return s.address
}

func (s *RarimoSecrets) PrivateKey() *secp256k1.PrivKey {
	return s.key
}

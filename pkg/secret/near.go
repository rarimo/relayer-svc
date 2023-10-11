package secret

import (
	"github.com/rarimo/near-go/common"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type NearSecrets struct {
	key     common.KeyPair
	address string
}

func nearSecretsFromMap(data map[string]interface{}) (*NearSecrets, error) {
	result := NearSecrets{}
	var err error
	strPk, ok := data[nearPrivateKeyKey].(string)
	if !ok {
		return nil, errors.New("invalid private key for the near chain")
	}

	result.key, err = common.NewBase58KeyPair(strPk)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse private key for the near chain")
	}

	result.address, ok = data[nearAddressKey].(string)
	if !ok {
		return nil, errors.New("invalid address for the near chain")
	}

	return &result, nil
}

func (s *NearSecrets) PublicKey() string {
	return s.address
}

func (s *NearSecrets) PrivateKey() common.KeyPair {
	return s.key
}

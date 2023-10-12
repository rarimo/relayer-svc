package secret

import (
	"context"
	"github.com/ethereum/go-ethereum/crypto"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/olegfomenko/solana-go"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

const (
	evmKey            = "evm"
	solanaKey         = "solana"
	nearKey           = "near"
	nearPrivateKeyKey = "private_key"
	rarimoKey         = "rarimo"
	nearAddressKey    = "address"
	bouncerKey        = "bouncer"
)

type Vault interface {
	Secret() *Secret
}

type vault struct {
	log      *logan.Entry
	secret   *Secret
	kvSecret *vaultapi.KVSecret
	client   *vaultapi.KVv2
	cfg      *VaultConfig
}

func (v *vault) Secret() *Secret {
	return v.secret
}

func (v *vault) loadSecret() error {
	var err error
	v.kvSecret, err = v.client.Get(context.Background(), v.cfg.Secret)
	if err != nil {
		return errors.Wrap(err, "failed to get secret data")
	}

	evm, err := evmSecretsFromMap(v.kvSecret.Data[evmKey].(map[string]interface{}))
	if err != nil {
		return errors.Wrap(err, "failed to parse evm secrets")
	}

	near, err := nearSecretsFromMap(v.kvSecret.Data[nearKey].(map[string]interface{}))
	if err != nil {
		return errors.Wrap(err, "failed to parse near secrets")
	}
	v.log.Info("[Vault] Near secrets key found")

	sol, err := solana.PrivateKeyFromBase58(v.kvSecret.Data[solanaKey].(string))
	if err != nil {
		return errors.Wrap(err, "valid base58-encoded solana private key expected")
	}
	v.log.Info("[Vault] Solana private key found")

	rarimo, err := rarimoSecretsFromMap(v.kvSecret.Data[rarimoKey].(string))
	if err != nil {
		return errors.Wrap(err, "expected a hex-encoded rarimo private key")
	}
	v.log.Info("[Vault] Rarimo private key found")

	bouncer, err := crypto.HexToECDSA(v.kvSecret.Data[bouncerKey].(string))
	if err != nil {
		return errors.Wrap(err, "expected a hex-encoded bouncer private key")
	}
	v.log.Info("[Vault] Bouncer private key found")

	v.secret, err = newSecret(evm, near, sol, rarimo, bouncer)
	if err != nil {
		return errors.Wrap(err, "failed to create secret")
	}

	v.log.Info("[Vault] Successfully initiated secret storage")

	return nil
}

package secret

import (
	vaultapi "github.com/hashicorp/vault/api"
	"gitlab.com/distributed_lab/figure/v3"
	"gitlab.com/distributed_lab/kit/comfig"
	"gitlab.com/distributed_lab/kit/kv"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type VaultConfig struct {
	Address string `fig:"address,required"`
	Mount   string `fig:"mount,required"`
	Secret  string `fig:"secret,required"`
	Token   string `fig:"token,required"`
}

type Vaulter interface {
	Vault() Vault
}

func NewVaulter(getter kv.Getter, log *logan.Entry) Vaulter {
	return &vaulter{
		getter: getter,
		log:    log,
	}
}

type vaulter struct {
	log    *logan.Entry
	getter kv.Getter
	comfig.Once
}

func (v *vaulter) Vault() Vault {
	return v.Do(func() interface{} {
		var config VaultConfig
		err := figure.
			Out(&config).
			From(kv.MustGetStringMap(v.getter, "vault")).
			Please()
		if err != nil {
			panic(errors.Wrap(err, "failed to figure out vault"))
		}

		conf := vaultapi.DefaultConfig()
		conf.Address = config.Address
		client, err := vaultapi.NewClient(conf)
		if err != nil {
			panic(errors.Wrap(err, "failed to create vault client"))
		}

		client.SetToken(config.Token)

		storage := &vault{
			cfg:    &config,
			client: client.KVv2(config.Mount),
			log:    v.log,
		}
		err = storage.loadSecret()
		if err != nil {
			panic(errors.Wrap(err, "failed to load secret"))
		}

		return storage
	}).(Vault)
}

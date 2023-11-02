package config

import (
	"gitlab.com/distributed_lab/figure/v3"
	"gitlab.com/distributed_lab/kit/comfig"
	"gitlab.com/distributed_lab/kit/kv"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type Rarimo struct {
	GasLimit    uint64 `fig:"gas_limit"`
	MinGasPrice uint64 `fig:"min_gas_price"`
	Coin        string `fig:"coin"`
	ChainID     string `fig:"chain_id"`
}

type Rarimoer interface {
	Rarimo() *Rarimo
}

type rarimoer struct {
	getter kv.Getter
	once   comfig.Once
}

func NewRarimoer(getter kv.Getter) Rarimoer {
	return &rarimoer{
		getter: getter,
	}
}

func (c *rarimoer) Rarimo() *Rarimo {
	return c.once.Do(func() interface{} {
		var cfg Rarimo

		if err := figure.Out(&cfg).From(kv.MustGetStringMap(c.getter, "rarimo")).Please(); err != nil {
			panic(errors.Wrap(err, "failed to load rarimo config"))
		}

		return &cfg
	}).(*Rarimo)
}

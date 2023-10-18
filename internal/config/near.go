package config

import (
	"github.com/rarimo/near-go/common"
	"github.com/rarimo/near-go/nearclient"
	"github.com/rarimo/near-go/nearprovider"
	"gitlab.com/distributed_lab/figure/v3"
	"gitlab.com/distributed_lab/kit/comfig"
	"gitlab.com/distributed_lab/kit/kv"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type nearer struct {
	getter kv.Getter
	once   comfig.Once
}

type Nearer interface {
	Near() *Near
}

func NewNearer(getter kv.Getter) Nearer {
	return &nearer{
		getter: getter,
	}
}

type Near struct {
	RPC           *nearclient.Client `fig:"rpc"`
	BridgeAddress common.AccountID   `fig:"bridge_address,required"`
}

func (n *nearer) Near() *Near {
	return n.once.Do(func() interface{} {
		var cfg Near

		err := figure.
			Out(&cfg).
			With(nearprovider.NearHooks, figure.BaseHooks).
			From(kv.MustGetStringMap(n.getter, "near")).
			Please()
		if err != nil {
			panic(errors.Wrap(err, "failed to figure config"))
		}

		return &cfg
	}).(*Near)
}

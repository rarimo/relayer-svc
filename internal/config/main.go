package config

import (
	"github.com/rarimo/relayer-svc/internal/data/horizon"
	"github.com/rarimo/relayer-svc/internal/data/redis"
	"github.com/rarimo/relayer-svc/pkg/bouncer"
	"github.com/rarimo/relayer-svc/pkg/secret"
	"gitlab.com/distributed_lab/kit/comfig"
	"gitlab.com/distributed_lab/kit/copus"
	"gitlab.com/distributed_lab/kit/copus/types"
	"gitlab.com/distributed_lab/kit/kv"
)

type Config interface {
	comfig.Logger
	types.Copuser
	comfig.Listenerer
	redis.Rediserer
	bouncer.Bouncerer
	horizon.Horizoner
	secret.Vaulter
	Tenderminter
	Cosmoser
	EVMer
	Solaner
	Nearer
	Schedulerer
}

type config struct {
	comfig.Logger
	types.Copuser
	comfig.Listenerer
	redis.Rediserer
	bouncer.Bouncerer
	horizon.Horizoner
	secret.Vaulter
	getter kv.Getter
	Tenderminter
	Cosmoser
	EVMer
	Solaner
	Nearer
	Schedulerer
}

func New(getter kv.Getter) Config {
	logger := comfig.NewLogger(getter, comfig.LoggerOpts{})
	return &config{
		Logger:       logger,
		getter:       getter,
		Copuser:      copus.NewCopuser(getter),
		Listenerer:   comfig.NewListenerer(getter),
		Rediserer:    redis.NewRediserer(getter, logger.Log()),
		Bouncerer:    bouncer.NewBouncerer(getter),
		Horizoner:    horizon.NewHorizoner(getter),
		Tenderminter: NewTenderminter(getter),
		Cosmoser:     NewCosmoser(getter),
		EVMer:        NewEVMer(getter),
		Solaner:      NewSolaner(getter),
		Nearer:       NewNearer(getter),
		Schedulerer:  NewSchedulerer(getter),
		Vaulter:      secret.NewVaulter(getter, logger.Log()),
	}
}

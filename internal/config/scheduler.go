package config

import (
	"gitlab.com/distributed_lab/figure/v3"
	"gitlab.com/distributed_lab/kit/comfig"
	"gitlab.com/distributed_lab/kit/kv"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type Schedulerer interface {
	Scheduler() *SchedulerConfig
}

type schedulerer struct {
	getter kv.Getter
	once   comfig.Once
}

type SchedulerConfig struct {
	StartBlock uint64 `fig:"start_block"`
}

func NewSchedulerer(getter kv.Getter) Schedulerer {
	return &schedulerer{
		getter: getter,
	}
}

func (c *schedulerer) Scheduler() *SchedulerConfig {
	return c.once.Do(func() interface{} {
		var cfg SchedulerConfig

		err := figure.
			Out(&cfg).
			From(kv.MustGetStringMap(c.getter, "scheduler")).
			Please()
		if err != nil {
			panic(errors.Wrap(err, "failed to parse scheduler config"))
		}

		return &cfg
	}).(*SchedulerConfig)
}

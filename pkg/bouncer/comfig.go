package bouncer

import (
	"github.com/pkg/errors"
	"gitlab.com/distributed_lab/figure/v3"
	"gitlab.com/distributed_lab/kit/comfig"
	"gitlab.com/distributed_lab/kit/kv"
	"time"
)

type Bouncerer interface {
	Bouncer() Bouncer
}

func NewBouncerer(getter kv.Getter) Bouncerer {
	return &bouncerer{
		getter: getter,
	}
}

type bouncerer struct {
	getter kv.Getter
	comfig.Once
}

func (b *bouncerer) Bouncer() Bouncer {
	return b.Do(func() interface{} {
		var config struct {
			SkipCheck bool `fig:"skip_checks"`
			TTL       int  `fig:"ttl,required"`
		}
		err := figure.
			Out(&config).
			From(kv.MustGetStringMap(b.getter, "bouncer")).
			Please()
		if err != nil {
			panic(errors.Wrap(err, "failed to figure out bouncer"))
		}
		return New(Config{
			SkipChecks: config.SkipCheck,
			TTL:        time.Duration(config.TTL) * time.Second,
		})
	}).(Bouncer)
}

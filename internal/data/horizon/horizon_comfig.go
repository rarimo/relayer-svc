package horizon

import (
	"gitlab.com/distributed_lab/figure/v3"
	"gitlab.com/distributed_lab/kit/comfig"
	"gitlab.com/distributed_lab/kit/kv"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"net/http"
	"time"
)

type horizoner struct {
	getter kv.Getter
	once   comfig.Once
}

type Horizoner interface {
	Horizon() Horizon
}

func NewHorizoner(getter kv.Getter) Horizoner {
	return &horizoner{
		getter: getter,
	}
}

func (h *horizoner) Horizon() Horizon {
	return h.once.Do(func() interface{} {
		var cfg horizon

		err := figure.
			Out(&cfg).
			From(kv.MustGetStringMap(h.getter, "horizon")).
			Please()
		if err != nil {
			panic(errors.Wrap(err, "failed to figure out horizon config"))
		}

		cfg.Client = &http.Client{
			Timeout: 2 * time.Minute,
		}

		return &cfg
	}).(*horizon)
}

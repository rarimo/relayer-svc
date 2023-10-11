package services

import (
	"context"
	"time"

	"github.com/rarimo/relayer-svc/internal/config"
	"github.com/rarimo/relayer-svc/internal/data/redis"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"gitlab.com/distributed_lab/running"
)

type queueCleaner struct {
	log   *logan.Entry
	redis redis.Rediser
}

func RunQueueCleaner(cfg config.Config, ctx context.Context) {
	log := cfg.Log().WithField("service", "queue_cleaner")
	q := queueCleaner{
		log:   log,
		redis: cfg.Redis(),
	}

	running.WithBackOff(ctx, log, "run_once", q.runOnce, 10*time.Minute, 10*time.Second, time.Minute)
}

func (q *queueCleaner) runOnce(ctx context.Context) error {
	stuck, err := q.redis.CleanQueues()
	if err != nil {
		return errors.Wrap(err, "failed to clean the redis queue")
	}

	ready, err := q.redis.OpenRelayQueue().PurgeReady()
	if err != nil {
		return errors.Wrap(err, "failed to clean the ready tasks")
	}

	rejected, err := q.redis.OpenRelayQueue().PurgeRejected()
	if err != nil {
		return errors.Wrap(err, "failed to clean the rejected tasks")
	}
	q.log.Infof("Cleaned %d stuck, %d ready, %d rejected jobs", stuck, ready, rejected)

	return nil
}

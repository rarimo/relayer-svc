package services

import (
	"context"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/status"

	"github.com/adjust/rmq/v5"
	"github.com/cosmos/cosmos-sdk/types/query"
	client "github.com/cosmos/cosmos-sdk/types/tx"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"gitlab.com/distributed_lab/running"
	"golang.org/x/exp/slices"

	rarimocore "github.com/rarimo/rarimo-core/x/rarimocore/types"
	"github.com/rarimo/relayer-svc/internal/config"
	"github.com/rarimo/relayer-svc/internal/data"
	"github.com/rarimo/relayer-svc/internal/data/core"
	"github.com/rarimo/relayer-svc/internal/services/relayer"
)

const (
	TxPerPageLimit       = 100
	BlockHeightCursorKey = "block_height_cursor"
	RunnerName           = "scheduler_catchup"
	InvalidHeightMessage = "codespace sdk code 26: invalid height"
)

type Scheduler interface {
	ScheduleRelays(
		ctx context.Context,
		confirmationID string,
		transferIndexes []string,
	) error
}

type scheduler struct {
	cfg        *config.SchedulerConfig
	log        *logan.Entry
	cosmos     client.ServiceClient
	core       core.Core
	relayQueue rmq.Queue
	redis      *redis.Client
}

func NewScheduler(cfg config.Config) Scheduler {
	return newScheduler(cfg)
}

func newScheduler(cfg config.Config) *scheduler {
	return &scheduler{
		log:        cfg.Log().WithField("service", "scheduler"),
		cosmos:     client.NewServiceClient(cfg.Cosmos()),
		relayQueue: cfg.Redis().OpenRelayQueue(),
		redis:      cfg.Redis().Client(),
		core:       core.NewCore(cfg),
		cfg:        cfg.Scheduler(),
	}
}

func RunScheduler(cfg config.Config, ctx context.Context) {
	s := newScheduler(cfg)
	s.log.Info("starting scheduler catchup")

	running.WithBackOff(ctx, s.log, RunnerName, func(ctx context.Context) error {
		cursor, err := s.getCursor(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get the cursor")
		}

		s.log.WithFields(logan.F{"cursor": cursor}).Debug("starting catchup")

		cursor, err = s.catchup(ctx, cursor)
		if err != nil {
			return errors.Wrap(err, "failed to catchup")
		}

		s.log.WithFields(logan.F{"cursor": cursor}).Debug("catchup finished")
		return nil
	}, 5*time.Second, 5*time.Second, 5*time.Second)
}

func (s *scheduler) catchup(ctx context.Context, cursor uint64) (uint64, error) {
	log := s.log.WithField("runner", "scheduler_catchup")

	log.Debug("starting catchup")

	currentCursor := cursor

	for {
		select {
		case <-ctx.Done():
			return cursor, ctx.Err()
		default:
			l := s.log.WithField("cursor", currentCursor)
			l.Debug("started processing block")

			txs, err := s.getTxsByBlockHeight(ctx, int64(currentCursor))
			if err != nil {
				if statusError, ok := status.FromError(errors.Cause(err)); ok {
					if strings.Contains(statusError.Message(), InvalidHeightMessage) {
						l.Debug("invalid height, waiting for the next block")
						return currentCursor, nil
					}
				}

				return cursor, errors.Wrap(err, "failed to get txs by block height")
			}

			for _, tx := range txs {
				for _, message := range tx.Body.Messages {
					if message.TypeUrl != "/rarimo.rarimocore.rarimocore.MsgCreateConfirmation" {
						continue
					}

					msg := rarimocore.MsgCreateConfirmation{}
					if err = msg.Unmarshal(message.Value); err != nil {
						log.WithError(err).Error("failed to unmarshal message")
						continue
					}

					if err := s.ScheduleRelays(ctx, msg.Root, msg.Indexes); err != nil {
						return currentCursor, errors.Wrap(err, "failed to schedule")
					}
				}
			}

			currentCursor++
			s.setCursor(ctx, currentCursor)
			l.Debug("finished processing block")
		}
	}
}

func (s *scheduler) getCursor(ctx context.Context) (uint64, error) {
	resp := s.redis.Get(ctx, BlockHeightCursorKey)
	if resp.Err() == redis.Nil {
		s.log.Debug("cursor not found in redis")

		if s.cfg.StartBlock != 0 {
			s.log.WithFields(logan.F{"cursor": s.cfg.StartBlock}).Debug("using start block from config")
			return s.cfg.StartBlock, nil
		}

		s.log.Debug("using first block as start block")
		return 1, nil
	}
	if resp.Err() != nil {
		return 0, errors.Wrap(resp.Err(), "failed to get cursor")
	}

	cursor, err := resp.Uint64()
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse the cursor value", logan.F{
			"raw": resp.String(),
		})
	}

	s.log.WithFields(logan.F{"cursor": cursor}).Debug("cursor found in redis")

	return cursor, nil
}

func (s *scheduler) getTxsByBlockHeight(ctx context.Context, height int64) ([]*client.Tx, error) {
	var txs []*client.Tx
	var nextKey []byte
	var stop = false

	for !stop {
		res, err := s.cosmos.GetBlockWithTxs(ctx, &client.GetBlockWithTxsRequest{Height: height, Pagination: &query.PageRequest{
			Key:   nextKey,
			Limit: TxPerPageLimit,
		}})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get block with txs")
		}

		nextKey = res.Pagination.NextKey
		stop = len(res.Pagination.NextKey) == 0
		txs = append(txs, res.Txs...)
	}

	return txs, nil
}

func (s *scheduler) setCursor(ctx context.Context, cursor uint64) {
	if resp := s.redis.Set(ctx, BlockHeightCursorKey, cursor, 0); resp.Err() != nil {
		panic(errors.Wrap(resp.Err(), "failed to set the cursor"))
	}
}

func (s *scheduler) ScheduleRelays(
	ctx context.Context,
	confirmationID string,
	transferIndexes []string,
) error {
	log := s.log.WithField("merkle_root", confirmationID)
	log.Info("processing a confirmation")

	transfers, err := s.core.GetTransfers(ctx, confirmationID)
	if err != nil {
		return errors.Wrap(err, "failed to get transfers")
	}

	var tasks []data.RelayTask
	for _, transfer := range transfers {
		if !slices.Contains(transferIndexes, transfer.Transfer.Origin) {
			continue
		}
		tasks = append(tasks, data.NewRelayTask(transfer, relayer.MaxRetries))
	}

	rawTasks := [][]byte{}
	for _, task := range tasks {
		if slices.Contains(transferIndexes, task.OperationIndex) {
			rawTasks = append(rawTasks, task.Marshal())
		}
	}

	if len(rawTasks) == 0 {
		log.Info("no transfers to relay")
		return nil
	}

	if err := s.relayQueue.PublishBytes(rawTasks...); err != nil {
		return errors.Wrap(err, "failed to publish tasks")
	}

	log.Infof("scheduled %d transfers for relay", len(rawTasks))

	return nil

}

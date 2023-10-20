package relayer

import (
	"context"
	"fmt"
	"time"

	"github.com/rarimo/relayer-svc/internal/services/bridger/bridge"

	"github.com/adjust/rmq/v5"

	rarimocore "github.com/rarimo/rarimo-core/x/rarimocore/types"
	tokenmanager "github.com/rarimo/rarimo-core/x/tokenmanager/types"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"

	"github.com/rarimo/relayer-svc/internal/config"
	"github.com/rarimo/relayer-svc/internal/data"
	"github.com/rarimo/relayer-svc/internal/data/core"
	"github.com/rarimo/relayer-svc/internal/services/bridger"
)

const (
	MaxRetries = 0

	prefetchLimit = 10
	pollDuration  = 100 * time.Millisecond
	numConsumers  = 100
)

type relayer struct {
	log   *logan.Entry
	queue rmq.Queue
}

type relayerConsumer struct {
	log             *logan.Entry
	rarimocore      rarimocore.QueryClient
	tokenmanager    tokenmanager.QueryClient
	bridgerProvider bridger.BridgerProvider
	queue           rmq.Queue
}

func Run(cfg config.Config, ctx context.Context) {
	log := cfg.Log().WithField("service", "relayer")
	r := relayer{
		log:   log,
		queue: cfg.Redis().OpenRelayQueue(),
	}

	if err := r.queue.StartConsuming(prefetchLimit, pollDuration); err != nil {
		panic(errors.Wrap(err, "failed to start consuming the relay queue"))
	}

	for i := 0; i < numConsumers; i++ {
		name := fmt.Sprintf("relay-consumer-%d", i)
		if _, err := r.queue.AddConsumer(name, newConsumer(cfg, name)); err != nil {
			panic(err)
		}
	}

	<-ctx.Done()
	<-r.queue.StopConsuming()
	r.log.Info("finished consuming relayer queue")
}

func newConsumer(cfg config.Config, id string) *relayerConsumer {
	return &relayerConsumer{
		log:             cfg.Log().WithField("service", id),
		rarimocore:      rarimocore.NewQueryClient(cfg.Cosmos()),
		tokenmanager:    tokenmanager.NewQueryClient(cfg.Cosmos()),
		queue:           cfg.Redis().OpenRelayQueue(),
		bridgerProvider: bridger.NewBridgerProvider(cfg),
	}
}

func (c *relayerConsumer) Consume(delivery rmq.Delivery) {
	defer func() {
		if err := recover(); err != nil {
			c.log.WithField("err", err).Error("relayer panicked")
		}
	}()

	var task data.RelayTask
	task.Unmarshal(delivery.Payload())

	if err := c.processTransfer(context.TODO(), task); err != nil {
		if errors.Cause(err) == bridge.ErrAlreadyWithdrawn {
			c.log.WithField("transfer_id", task.OperationIndex).Info("transfer was already withdrawn")
			return
		}

		c.log.WithError(err).WithField("transfer_id", task.OperationIndex).Error("failed to process transfer")
		mustReject(delivery)
		c.mustScheduleRetry(task)
		return
	}

	if err := delivery.Ack(); err != nil {
		panic(errors.Wrap(err, fmt.Sprintf("failed to ack the transfer %s", task.OperationIndex)))
	}
}

func (c *relayerConsumer) processTransfer(ctx context.Context, task data.RelayTask) error {
	log := c.log.WithField("op_id", task.OperationIndex)

	log.Info("processing a transfer")
	operation, err := c.rarimocore.Operation(ctx, &rarimocore.QueryGetOperationRequest{Index: task.OperationIndex})
	if err != nil {
		return errors.Wrap(err, "failed to get the transfer")
	}
	if operation.Operation.Status != rarimocore.OpStatus_SIGNED {
		return errors.New("transfer is not signed yet")
	}
	transfer := rarimocore.Transfer{}
	if err := transfer.Unmarshal(operation.Operation.Details.Value); err != nil {
		return errors.Wrap(err, "failed to unmarshal  transfer")
	}

	tokenDetails, err := c.tokenmanager.ItemByOnChainItem(ctx, &tokenmanager.QueryGetItemByOnChainItemRequest{
		Address: transfer.To.Address,
		TokenID: transfer.To.TokenID,
		Chain:   transfer.To.Chain,
	})
	if err != nil {
		return errors.Wrap(err, "failed to get token details")
	}

	collection, err := c.tokenmanager.Collection(ctx, &tokenmanager.QueryGetCollectionRequest{
		Index: tokenDetails.Item.Collection,
	})
	if err != nil {
		return errors.Wrap(err, "failed to get collection")
	}

	collectionData, err := c.tokenmanager.CollectionDataByCollectionForChain(ctx, &tokenmanager.QueryGetCollectionDataByCollectionForChainRequest{
		Chain:           transfer.To.Chain,
		CollectionIndex: collection.Collection.Index,
	})
	if err != nil {
		return errors.Wrap(err, "failed to get collection data")
	}

	transferDetails := core.TransferDetails{
		Transfer:       transfer,
		Collection:     collection.Collection,
		CollectionData: collectionData.Data,
		Item:           tokenDetails.Item,
		Signature:      task.Signature,
		Origin:         task.Origin,
		MerklePath:     task.MustParseMerklePath(),
	}

	f := logan.F{
		"to":         transfer.Receiver,
		"token_type": collectionData.Data.TokenType,
		"to_chain":   transfer.To.Chain,
	}

	log.WithFields(f).Info("relaying a transfer")

	return c.bridgerProvider.GetBridger(transfer.To.Chain).Withdraw(ctx, transferDetails)
}

func mustReject(delivery rmq.Delivery) {
	if err := delivery.Reject(); err != nil {
		panic(errors.Wrap(err, "failed to reject the task"))
	}
}

func (c *relayerConsumer) mustScheduleRetry(task data.RelayTask) {
	/**
	TODO:
		- add exponential backoff
		- distinguish retryable and non-retryable errors
		- set up a dead letter queue
	*/
	if task.RetriesLeft == 0 {
		return
	}

	task.RetriesLeft--
	if err := c.queue.PublishBytes(task.Marshal()); err != nil {
		panic(errors.Wrap(err, "failed to schedule the retry"))
	}

}

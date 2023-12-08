package core

import (
	"context"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/rarimo/rarimo-core/x/rarimocore/crypto/pkg"
	rarimocore "github.com/rarimo/rarimo-core/x/rarimocore/types"
	tokenmanager "github.com/rarimo/rarimo-core/x/tokenmanager/types"
	"github.com/rarimo/relayer-svc/internal/config"
	"github.com/rarimo/relayer-svc/internal/utils"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"golang.org/x/exp/slices"
)

type core struct {
	log  *logan.Entry
	core rarimocore.QueryClient
	tm   tokenmanager.QueryClient
}

type Core interface {
	GetTransfers(ctx context.Context, confirmationID string) ([]TransferDetails, error)
	GetTransfer(ctx context.Context, confirmationID string, transferID string) (*TransferDetails, error)
	GetConfirmation(ctx context.Context, confirmationID string) (*rarimocore.Confirmation, error)
}

func NewCore(cfg config.Config) Core {
	return &core{
		core: rarimocore.NewQueryClient(cfg.Cosmos()),
		tm:   tokenmanager.NewQueryClient(cfg.Cosmos()),
		log:  cfg.Log().WithField("service", "core"),
	}
}

func (c *core) GetConfirmation(ctx context.Context, confirmationID string) (*rarimocore.Confirmation, error) {
	confirmation, err := c.core.Confirmation(ctx, &rarimocore.QueryGetConfirmationRequest{
		Root: confirmationID,
	})

	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch the confirmation")
	}
	if confirmation == nil {
		return nil, errors.New("confirmation not found")
	}

	return &confirmation.Confirmation, nil
}

func (c *core) GetTransfer(ctx context.Context, confirmationID string, transferID string) (*TransferDetails, error) {
	transfers, err := c.GetTransfers(ctx, confirmationID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get transfers")
	}
	transferI := slices.IndexFunc(transfers, func(t TransferDetails) bool {
		return t.Origin == transferID
	})
	if transferI == -1 {
		return nil, errors.New("transfer not found")
	}

	return &transfers[transferI], nil
}

func (c *core) GetTransfers(ctx context.Context, confirmationID string) ([]TransferDetails, error) {
	log := c.log.WithField("merkle_root", confirmationID)
	log.Info("processing a confirmation")

	confirmation, err := c.GetConfirmation(ctx, confirmationID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch the confirmation")
	}
	params, err := c.tm.Params(ctx, new(tokenmanager.QueryParamsRequest))
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch the network params")
	}

	var transfers []TransferDetails

	for _, id := range confirmation.Indexes {
		op, err := c.core.Operation(ctx, &rarimocore.QueryGetOperationRequest{Index: id})
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch the operation", logan.F{
				"index": id,
			})
		}

		if op.Operation.OperationType != rarimocore.OpType_TRANSFER {
			continue
		}

		transfer, err := c.getTransferDetails(ctx, params.Params, confirmation.SignatureECDSA, op.Operation)

		proof, err := c.core.OperationProof(ctx, &rarimocore.QueryGetOperationProofRequest{Index: id})
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch the operation proof", logan.F{
				"index": id,
			})
		}

		transfer.MerklePath = make([][32]byte, len(proof.Path))

		for i, hash := range proof.Path {
			transfer.MerklePath[i] = utils.ToByte32(hexutil.MustDecode(hash))
		}

		transfers = append(transfers, *transfer)
	}

	return transfers, nil
}

func (c *core) getTransferDetails(ctx context.Context, params tokenmanager.Params, signature string, op rarimocore.Operation) (*TransferDetails, error) {
	transfer := rarimocore.Transfer{}
	if err := transfer.Unmarshal(op.Details.Value); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal the transfer", logan.F{
			"op_id": op.Index,
		})
	}

	tokenDetails, err := c.tm.ItemByOnChainItem(ctx, &tokenmanager.QueryGetItemByOnChainItemRequest{
		Address: transfer.To.Address,
		TokenID: transfer.To.TokenID,
		Chain:   transfer.To.Chain,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get token details", logan.F{
			"address": transfer.To.Address,
			"tokenID": transfer.To.TokenID,
			"chain":   transfer.To.Chain,
			"op_id":   op.Index,
		})
	}

	collection, err := c.tm.Collection(ctx, &tokenmanager.QueryGetCollectionRequest{
		Index: tokenDetails.Item.Collection,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get collection", logan.F{
			"index": tokenDetails.Item.Collection,
			"op_id": op.Index,
		})
	}

	collectionData, err := c.tm.CollectionDataByCollectionForChain(ctx, &tokenmanager.QueryGetCollectionDataByCollectionForChainRequest{
		Chain:           transfer.To.Chain,
		CollectionIndex: collection.Collection.Index,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get collection data by collection for chain", logan.F{
			"chain":           transfer.To.Chain,
			"collectionIndex": collection.Collection.Index,
			"op_id":           op.Index,
		})
	}

	var bridgeParams *tokenmanager.BridgeNetworkParams
	for _, network := range params.Networks {
		if network.Name == transfer.To.Chain {
			bridgeParams = network.GetBridgeParams()
		}
	}

	if bridgeParams == nil {
		return nil, errors.From(errors.New("bridge params not found"), logan.F{
			"chain": transfer.To.Chain,
			"op_id": op.Index,
		})
	}

	content, err := pkg.GetTransferContent(
		collection.Collection,
		collectionData.Data,
		tokenDetails.Item,
		bridgeParams,
		&transfer,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get transfer content", logan.F{
			"op_id": op.Index,
		})
	}

	return &TransferDetails{
		Transfer:       transfer,
		Collection:     collection.Collection,
		CollectionData: collectionData.Data,
		Item:           tokenDetails.Item,
		Signature:      signature,
		Origin:         hexutil.Encode(content.Origin[:]),
	}, nil
}

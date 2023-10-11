package core

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	merkle "github.com/rarimo/go-merkle"
	"github.com/rarimo/rarimo-core/x/rarimocore/crypto/operation"
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

	transfers := make([]TransferDetails, 0, len(confirmation.Indexes))
	operations := []*operation.TransferContent{}
	contents := []merkle.Content{}

	for _, id := range confirmation.Indexes {
		operation, err := c.core.Operation(ctx, &rarimocore.QueryGetOperationRequest{Index: id})
		if err != nil {
			return nil, errors.Wrap(err, "failed to fetch the operation")
		}
		transfer := rarimocore.Transfer{}
		if err := transfer.Unmarshal(operation.Operation.Details.Value); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal the transfer")
		}

		tokenDetails, err := c.tm.ItemByOnChainItem(ctx, &tokenmanager.QueryGetItemByOnChainItemRequest{
			Address: transfer.To.Address,
			TokenID: transfer.To.TokenID,
			Chain:   transfer.To.Chain,
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get token details")
		}

		collection, err := c.tm.Collection(ctx, &tokenmanager.QueryGetCollectionRequest{
			Index: tokenDetails.Item.Collection,
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get collection")
		}

		collectionData, err := c.tm.CollectionDataByCollectionForChain(ctx, &tokenmanager.QueryGetCollectionDataByCollectionForChainRequest{
			Chain:           transfer.To.Chain,
			CollectionIndex: collection.Collection.Index,
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get collection data")
		}

		var bridgeParams *tokenmanager.BridgeNetworkParams
		for _, network := range params.Params.Networks {
			if network.Name == transfer.To.Chain {
				bridgeParams = network.GetBridgeParams()
			}
		}

		if bridgeParams == nil {
			return nil, errors.New("bridge params not found")
		}

		content, err := pkg.GetTransferContent(
			collection.Collection,
			collectionData.Data,
			tokenDetails.Item,
			bridgeParams,
			&transfer,
		)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get transfer content")
		}
		contents = append(contents, content)
		operations = append(operations, content)

		transfers = append(transfers, TransferDetails{
			Transfer:       transfer,
			Collection:     collection.Collection,
			CollectionData: collectionData.Data,
			Item:           tokenDetails.Item,
			Signature:      confirmation.SignatureECDSA,
			Origin:         hexutil.Encode(content.Origin[:]),
		})
	}

	tree := merkle.NewTree(crypto.Keccak256, contents...)
	for i, operation := range operations {
		rawPath, ok := tree.Path(operation)
		if !ok {
			panic(fmt.Errorf("failed to build Merkle tree"))
		}
		transfers[i].MerklePath = make([][32]byte, 0, len(rawPath))
		for _, hash := range rawPath {
			transfers[i].MerklePath = append(transfers[i].MerklePath, utils.ToByte32(hash))
		}
	}

	return transfers, nil
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

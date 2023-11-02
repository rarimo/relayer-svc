package near

import (
	"context"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/rarimo/near-go/common"
	"github.com/rarimo/near-go/nearclient"
	tokenmanager "github.com/rarimo/rarimo-core/x/tokenmanager/types"
	"github.com/rarimo/relayer-svc/internal/config"
	"github.com/rarimo/relayer-svc/internal/data/core"
	"github.com/rarimo/relayer-svc/internal/data/horizon"
	"github.com/rarimo/relayer-svc/internal/services/bridger/bridge"
	"github.com/rarimo/relayer-svc/internal/utils"
	"github.com/rarimo/relayer-svc/pkg/secret"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"lukechampine.com/uint128"
	"math/big"
)

type nearBridger struct {
	log     *logan.Entry
	near    *config.Near
	vault   secret.Vault
	horizon horizon.Horizon
}

func NewNearBridger(cfg config.Config) bridge.Bridger {
	return &nearBridger{
		log:     cfg.Log().WithField("service", "near_bridge"),
		near:    cfg.Near(),
		vault:   cfg.Vault(),
		horizon: cfg.Horizon(),
	}
}

func (b *nearBridger) Withdraw(
	ctx context.Context,
	transfer core.TransferDetails,
) error {
	log := b.log.WithField("op_id", transfer.Origin)

	amount, err := parseNearAmount(transfer.Transfer.Amount)
	if err != nil {
		return errors.Wrap(err, "failed to parse amount")
	}
	rawSignature := hexutil.MustDecode(transfer.Signature)
	signature := hexutil.Encode(rawSignature[:64])

	withdrawArgs := common.WithdrawArgs{
		SignArgs: common.SignArgs{
			Origin:     transfer.Origin,
			Path:       transfer.MerklePath,
			Signature:  signature,
			RecoveryID: rawSignature[64],
		},
		ReceiverID: string(hexutil.MustDecode(transfer.Transfer.Receiver)),
	}

	var act common.Action

	isWrapped := transfer.CollectionData.Wrapped
	tokenAddress := transfer.Transfer.To.Address

	switch transfer.CollectionData.TokenType {
	case tokenmanager.Type_NATIVE:
		args := common.NativeWithdrawArgs{
			Amount:       amount,
			WithdrawArgs: withdrawArgs,
		}
		act = common.NewNativeWithdrawCall(args, common.DefaultFunctionCallGas, common.OneYocto)
	case tokenmanager.Type_NEAR_FT:
		args := common.FtWithdrawArgs{
			Token:        string(hexutil.MustDecode(tokenAddress)),
			Amount:       amount,
			IsWrapped:    isWrapped,
			WithdrawArgs: withdrawArgs,
		}

		act = common.NewFtWithdrawCall(args, common.DefaultFunctionCallGas, common.FtMintStorageDeposit)
	case tokenmanager.Type_NEAR_NFT:
		deposit := common.OneYocto
		args := common.NftWithdrawArgs{
			Token:        string(hexutil.MustDecode(tokenAddress)),
			TokenID:      string(hexutil.MustDecode(transfer.Transfer.To.TokenID)),
			IsWrapped:    isWrapped,
			WithdrawArgs: withdrawArgs,
		}
		if isWrapped {
			metadata, err := b.horizon.NftMetadata(
				transfer.Transfer.To.Chain,
				transfer.Item.Index,
				transfer.Transfer.To.TokenID,
			)
			if err != nil {
				return errors.Wrap(err, "failed to get NFT metadata")
			}

			args.TokenMetadata = toNearNftMetadata(metadata, transfer.Item.Meta)
			deposit = common.NftMintStorageDeposit
		}

		act = common.NewNftWithdrawCall(args, common.DefaultFunctionCallGas, deposit)
	default:
		return errors.Errorf("invalid near token type: %d", transfer.CollectionData.TokenType)
	}

	withdrawResp, err := b.near.RPC.TransactionSendAwait(
		nearclient.ContextWithKeyPair(ctx, b.vault.Secret().Near().PrivateKey()),
		b.vault.Secret().Near().PublicKey(),
		b.near.BridgeAddress,
		[]common.Action{act},
		nearclient.WithLatestBlock(),
	)
	if err != nil {
		return errors.Wrap(err, "failed to submit a Near transaction")
	}
	if len(withdrawResp.Status.Failure) != 0 {
		log.
			WithField("tx_id", withdrawResp.Transaction.Hash).
			WithField("status_failure", utils.Prettify(withdrawResp.Status.Failure)).
			Info("near transaction failed")

		return errors.New("near transaction failed")
	}

	log.WithField("tx_id", withdrawResp.Transaction.Hash).Info("successfully submitted Near transaction")

	return nil
}

func parseNearAmount(raw string) (common.Balance, error) {
	bigAmount, err := utils.GetAmountOrDefault(raw, big.NewInt(1))
	if err != nil {
		return common.Balance{}, errors.Wrap(err, "failed to parse amount")
	}

	return common.Balance(uint128.FromBig(bigAmount)), nil
}

func toNearNftMetadata(horizonMeta *horizon.NftMetadata, coreMeta tokenmanager.ItemMetadata) *common.NftMetadataView {
	res := common.NftMetadataView{
		Title:     horizonMeta.Name,
		Media:     horizonMeta.ImageUrl,
		Reference: horizonMeta.MetadataUrl,
		Copies:    1,
	}

	if horizonMeta.Description != nil {
		res.Description = *horizonMeta.Description
	}
	if coreMeta.ImageHash != "" {
		res.MediaHash = hexutil.MustDecode(coreMeta.ImageHash)
	}

	return &res
}

package relayer

import (
	"context"
	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/types"
	client "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	xauthsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	bridgetypes "github.com/rarimo/rarimo-core/x/bridge/types"
	tokenmanager "github.com/rarimo/rarimo-core/x/tokenmanager/types"
	"github.com/rarimo/relayer-svc/internal/data"
	"github.com/rarimo/relayer-svc/internal/data/core"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

func (c *relayerConsumer) processRarimoTransfer(
	ctx context.Context,
	task data.RelayTask,
	transferDetails core.TransferDetails,
) error {
	f := logan.F{"op_id": task.OperationIndex}

	if transferDetails.CollectionData.TokenType != tokenmanager.Type_NATIVE {
		return errors.From(errors.New("only native tokens are supported"), f)
	}
	builder := c.txConfig.NewTxBuilder()
	address := c.vault.Secret().Rarimo().PublicKey()

	err := builder.SetMsgs(&bridgetypes.MsgWithdrawNative{
		Creator: c.vault.Secret().Rarimo().PublicKey(),
		Origin:  transferDetails.Origin,
	})
	if err != nil {
		return errors.Wrap(err, "failed to set withdraw message to the tx builder", f)
	}

	builder.SetGasLimit(c.rarimo.GasLimit)
	builder.SetFeeAmount(types.Coins{types.NewInt64Coin(c.rarimo.Coin, int64(c.rarimo.GasLimit*c.rarimo.MinGasPrice))})

	accountResp, err := c.auth.Account(ctx, &authtypes.QueryAccountRequest{Address: address})
	if err != nil {
		return errors.Wrap(err, "failed to get account", f)
	}

	account := authtypes.BaseAccount{}
	err = account.Unmarshal(accountResp.Account.Value)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal account", f)
	}

	accountSequence := account.GetSequence()

	err = builder.SetSignatures(signing.SignatureV2{
		PubKey: c.vault.Secret().Rarimo().PrivateKey().PubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  c.txConfig.SignModeHandler().DefaultMode(),
			Signature: nil,
		},
		Sequence: accountSequence,
	})
	if err != nil {
		return errors.Wrap(err, "failed to set signature to the tx builder", f)
	}

	signerData := xauthsigning.SignerData{
		ChainID:       c.rarimo.ChainID,
		AccountNumber: account.AccountNumber,
		Sequence:      accountSequence,
	}

	sigV2, err := clienttx.SignWithPrivKey(
		c.txConfig.SignModeHandler().DefaultMode(), signerData,
		builder, c.vault.Secret().Rarimo().PrivateKey(), c.txConfig, accountSequence,
	)

	err = builder.SetSignatures(sigV2)
	if err != nil {
		return errors.Wrap(err, "failed to set signature v2 to the tx builder", f)
	}

	tx, err := c.txConfig.TxEncoder()(builder.GetTx())
	if err != nil {
		return errors.Wrap(err, "failed to encode tx", f)
	}

	resp, err := c.tx.BroadcastTx(
		ctx,
		&client.BroadcastTxRequest{
			Mode:    client.BroadcastMode_BROADCAST_MODE_BLOCK,
			TxBytes: tx,
		},
	)
	if err != nil {
		return errors.Wrap(err, "failed to broadcast tx", f)
	}

	c.log.WithFields(f.Merge(logan.F{
		"tx_id": resp.TxResponse.TxHash,
	})).Info("successfully submitted Rarimo transaction")

	return nil
}

package rarimo

import (
	"context"
	clientypes "github.com/cosmos/cosmos-sdk/client"
	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/types"
	client "github.com/cosmos/cosmos-sdk/types/tx"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	xauthsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	bridgetypes "github.com/rarimo/rarimo-core/x/bridge/types"
	tokenmanager "github.com/rarimo/rarimo-core/x/tokenmanager/types"
	"github.com/rarimo/relayer-svc/internal/config"
	"github.com/rarimo/relayer-svc/internal/data/core"
	"github.com/rarimo/relayer-svc/internal/services/bridger/bridge"
	"github.com/rarimo/relayer-svc/pkg/secret"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type rarimoBridger struct {
	log      *logan.Entry
	vault    secret.Vault
	rarimo   *config.Rarimo
	txConfig clientypes.TxConfig
	auth     authtypes.QueryClient
	tx       sdktx.ServiceClient
}

func NewRarimoBridger(cfg config.Config) bridge.Bridger {
	return &rarimoBridger{
		log:      cfg.Log().WithField("service", "rarimo_bridge"),
		vault:    cfg.Vault(),
		rarimo:   cfg.Rarimo(),
		txConfig: tx.NewTxConfig(codec.NewProtoCodec(codectypes.NewInterfaceRegistry()), []signing.SignMode{signing.SignMode_SIGN_MODE_DIRECT}),
		auth:     authtypes.NewQueryClient(cfg.Cosmos()),
		tx:       sdktx.NewServiceClient(cfg.Cosmos()),
	}
}

func (b *rarimoBridger) Withdraw(
	ctx context.Context,
	transfer core.TransferDetails,
) error {
	f := logan.F{"op_id": transfer.Origin}

	if transfer.CollectionData.TokenType != tokenmanager.Type_NATIVE {
		return errors.From(errors.New("only native tokens are supported"), f)
	}
	builder := b.txConfig.NewTxBuilder()
	address := b.vault.Secret().Rarimo().PublicKey()

	err := builder.SetMsgs(&bridgetypes.MsgWithdrawNative{
		Creator: address,
		Origin:  transfer.Origin,
	})
	if err != nil {
		return errors.Wrap(err, "failed to set withdraw message to the tx builder", f)
	}

	builder.SetGasLimit(b.rarimo.GasLimit)
	builder.SetFeeAmount(types.Coins{types.NewInt64Coin(b.rarimo.Coin, int64(b.rarimo.GasLimit*b.rarimo.MinGasPrice))})

	accountResp, err := b.auth.Account(ctx, &authtypes.QueryAccountRequest{Address: address})
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
		PubKey: b.vault.Secret().Rarimo().PrivateKey().PubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  b.txConfig.SignModeHandler().DefaultMode(),
			Signature: nil,
		},
		Sequence: accountSequence,
	})
	if err != nil {
		return errors.Wrap(err, "failed to set signature to the tx builder", f)
	}

	signerData := xauthsigning.SignerData{
		ChainID:       b.rarimo.ChainID,
		AccountNumber: account.AccountNumber,
		Sequence:      accountSequence,
	}

	sigV2, err := clienttx.SignWithPrivKey(
		b.txConfig.SignModeHandler().DefaultMode(), signerData,
		builder, b.vault.Secret().Rarimo().PrivateKey(), b.txConfig, accountSequence,
	)

	err = builder.SetSignatures(sigV2)
	if err != nil {
		return errors.Wrap(err, "failed to set signature v2 to the tx builder", f)
	}

	tx, err := b.txConfig.TxEncoder()(builder.GetTx())
	if err != nil {
		return errors.Wrap(err, "failed to encode tx", f)
	}

	resp, err := b.tx.BroadcastTx(
		ctx,
		&client.BroadcastTxRequest{
			Mode:    client.BroadcastMode_BROADCAST_MODE_BLOCK,
			TxBytes: tx,
		},
	)
	if err != nil {
		return errors.Wrap(err, "failed to broadcast tx", f)
	}

	b.log.WithFields(f.Merge(logan.F{
		"tx_id": resp.TxResponse.TxHash,
	})).Info("successfully submitted Rarimo transaction")

	return nil
}

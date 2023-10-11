package solana

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/olegfomenko/solana-go"
	"github.com/olegfomenko/solana-go/rpc"
	confirm "github.com/olegfomenko/solana-go/rpc/sendAndConfirmTransaction"
	"github.com/pkg/errors"
	tokenmanager "github.com/rarimo/rarimo-core/x/tokenmanager/types"
	"github.com/rarimo/relayer-svc/internal/config"
	"github.com/rarimo/relayer-svc/internal/data/core"
	"github.com/rarimo/relayer-svc/internal/services/bridger/bridge"
	"github.com/rarimo/relayer-svc/internal/utils"
	"github.com/rarimo/relayer-svc/pkg/secret"
	solanabridge "github.com/rarimo/solana-program-go/contracts/bridge"
	"gitlab.com/distributed_lab/logan/v3"
	"math/big"
)

type solanaBridger struct {
	log          *logan.Entry
	tokenmanager tokenmanager.QueryClient
	solana       *config.Solana
	vault        secret.Vault
}

func NewSolanaBridger(cfg config.Config) bridge.Bridger {
	return &solanaBridger{
		log:          cfg.Log().WithField("service", "solana_bridger"),
		tokenmanager: tokenmanager.NewQueryClient(cfg.Cosmos()),
		solana:       cfg.Solana(),
	}
}

func (b *solanaBridger) Withdraw(
	ctx context.Context,
	transfer core.TransferDetails,
) error {
	log := b.log.WithField("op_id", transfer.Origin)
	withdrawn, err := b.isAlreadyWithdrawn(ctx, transfer)
	if err != nil {
		return errors.Wrap(err, "failed to check if the transfer is withdrawn")
	}
	if withdrawn {
		return bridge.ErrAlreadyWithdrawn
	}

	tx, err := b.makeWitdrawTx(ctx, transfer)
	if err != nil {
		return errors.Wrap(err, "failed to call the withdraw method")
	}
	sig, err := confirm.SendAndConfirmTransaction(
		ctx,
		b.solana.RPC,
		b.solana.WS,
		tx,
	)
	if err != nil {
		return errors.Wrap(err, "failed to submit a solana transaction")
	}

	log.WithFields(logan.F{"sig": sig.String()}).Info("successfully submitted transaction")

	return nil
}

func (b *solanaBridger) makeWitdrawTx(
	ctx context.Context,
	transferDetails core.TransferDetails,
) (*solana.Transaction, error) {
	receiver := hexutil.MustDecode(transferDetails.Transfer.Receiver)
	origin := utils.ToByte32(hexutil.MustDecode(transferDetails.Origin))
	signature := hexutil.MustDecode(transferDetails.Signature)
	amount, err := utils.GetAmountOrDefault(transferDetails.Transfer.Amount, big.NewInt(1))
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("invalid amount: %s", transferDetails.Transfer.Amount))
	}

	args := solanabridge.WithdrawArgs{
		Amount:     amount.Uint64(),
		Path:       transferDetails.MerklePath,
		RecoveryId: signature[64],
		Seeds:      b.solana.BridgeAdminSeed,
		Origin:     origin,
		Signature:  utils.ToByte64(signature),
	}

	withdrawAddress, _, err := solana.FindProgramAddress([][]byte{origin[:]}, b.solana.BridgeProgramID)
	if err != nil {
		return nil, errors.New("failed to create withdraw address")
	}

	var instruction solana.Instruction
	switch transferDetails.CollectionData.TokenType {
	case tokenmanager.Type_NATIVE:
		instruction, err = solanabridge.WithdrawNativeInstruction(
			b.solana.BridgeProgramID,
			b.solana.BridgeAdmin,
			solana.PublicKeyFromBytes(receiver),
			withdrawAddress,
			args,
		)
	case tokenmanager.Type_METAPLEX_FT:
		tokenAddress := hexutil.MustDecode(transferDetails.Transfer.To.Address)
		instruction, err = solanabridge.WithdrawFTInstruction(
			b.solana.BridgeProgramID,
			b.solana.BridgeAdmin,
			solana.PublicKeyFromBytes(tokenAddress),
			solana.PublicKeyFromBytes(receiver),
			withdrawAddress,
			args,
		)
	case tokenmanager.Type_METAPLEX_NFT:
		tokenID := hexutil.MustDecode(transferDetails.Transfer.To.TokenID)
		instruction, err = solanabridge.WithdrawNFTInstruction(
			b.solana.BridgeProgramID,
			b.solana.BridgeAdmin,
			solana.PublicKeyFromBytes(tokenID),
			solana.PublicKeyFromBytes(receiver),
			withdrawAddress,
			args,
		)
	default:
		return nil, errors.Errorf("invalid solana token type: %d", transferDetails.CollectionData.TokenType)
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to construct the solana instruction")
	}

	recent, err := b.solana.RPC.GetLatestBlockhash(
		context.Background(),
		rpc.CommitmentFinalized,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch recent blockhash")
	}

	tx, err := solana.NewTransaction(
		[]solana.Instruction{instruction},
		recent.Value.Blockhash,
		solana.TransactionPayer(b.vault.Secret().Solana().PublicKey()),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to form a solana transaction")
	}

	if _, err = tx.AddSignature(b.vault.Secret().Solana().PrivateKey()); err != nil {
		return nil, errors.Wrap(err, "failed to sign a solana transaction")
	}

	return tx, nil
}

func (b solanaBridger) isAlreadyWithdrawn(ctx context.Context, transfer core.TransferDetails) (bool, error) {
	origin := utils.ToByte32(hexutil.MustDecode(transfer.Origin))
	withdrawAddress, _, err := solana.FindProgramAddress([][]byte{origin[:]}, b.solana.BridgeProgramID)
	if err != nil {
		return false, errors.New("failed to create withdraw address")
	}
	_, err = b.solana.RPC.GetAccountInfoWithOpts(
		ctx, withdrawAddress,
		&rpc.GetAccountInfoOpts{
			Commitment: rpc.CommitmentType(rpc.ConfirmationStatusProcessed),
		},
	)
	if errors.Cause(err) == rpc.ErrNotFound {
		// has not been withdrawn yet
	} else if err != nil {
		return false, errors.Wrap(err, "failed to get withdraw account")
	} else {
		return true, nil
	}

	return false, nil
}

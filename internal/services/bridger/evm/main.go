package evm

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	bridgebind "github.com/rarimo/evm-bridge-contracts/gobind/contracts/bridge"
	facadebind "github.com/rarimo/evm-bridge-contracts/gobind/contracts/interfaces/facade"
	rarimocore "github.com/rarimo/rarimo-core/x/rarimocore/types"
	tokenmanager "github.com/rarimo/rarimo-core/x/tokenmanager/types"
	"github.com/rarimo/relayer-svc/internal/config"
	"github.com/rarimo/relayer-svc/internal/data/core"
	"github.com/rarimo/relayer-svc/internal/services/bridger/bridge"
	"github.com/rarimo/relayer-svc/internal/utils"
	"github.com/rarimo/relayer-svc/pkg/secret"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"math/big"
)

type evmBridger struct {
	log          *logan.Entry
	tokenmanager tokenmanager.QueryClient
	evm          *config.EVM
	vault        secret.Vault
}

func NewEVMBridger(cfg config.Config) bridge.Bridger {
	return &evmBridger{
		log:          cfg.Log().WithField("service", "evm_bridge"),
		tokenmanager: tokenmanager.NewQueryClient(cfg.Cosmos()),
		evm:          cfg.EVM(),
	}
}

func (b *evmBridger) makeWithdrawTx(
	ctx context.Context,
	chain *config.EVMChain,
	transfer core.TransferDetails,
	simulation bool,
) (*types.Transaction, error) {
	bridgeFacade, err := facadebind.NewIBridgeFacade(chain.BridgeFacadeAddress, chain.RPC)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make an instance of the ethereum bridge facade")
	}

	amount, err := utils.GetAmountOrDefault(transfer.Transfer.Amount, big.NewInt(1))
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("invalid amount: %s", transfer.Transfer.Amount))
	}
	receiver := common.HexToAddress(transfer.Transfer.Receiver)
	origin := utils.ToByte32(hexutil.MustDecode(transfer.Origin))
	/**
	Tweak the V value to make it compatible with the OpenZeppelin ECDSA implementation
	https://github.com/OpenZeppelin/openzeppelin-contracts/blob/a1948250ab8c441f6d327a65754cb20d2b1b4554/contracts/utils/cryptography/ECDSA.sol#L143
	*/
	signature := hexutil.MustDecode(transfer.Signature)
	signature[64] += 27

	proof, err := proofABI.Pack(transfer.MerklePath, signature)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ABI encode the proof")
	}
	bundle, err := getBundleData(transfer.Transfer)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse bundle data")
	}

	opts, err := bind.NewKeyedTransactorWithChainID(b.vault.Secret().EVM().PrivateKey(chain.Name), chain.ChainID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a bridge transactor")
	}

	opts.Context = ctx
	opts.NoSend = simulation
	nonce, err := chain.RPC.PendingNonceAt(ctx, b.vault.Secret().EVM().PublicKey(chain.Name))
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch a nonce")
	}
	opts.Nonce = big.NewInt(int64(nonce))
	gasPrice, err := chain.RPC.SuggestGasPrice(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get suggested gas price")
	}
	opts.GasPrice = gasPrice
	opts.GasLimit = uint64(1000000) // TODO: estimate or make it configurable

	switch transfer.CollectionData.TokenType {
	case tokenmanager.Type_NATIVE:
		return bridgeFacade.WithdrawNative(
			opts,
			facadebind.INativeHandlerWithdrawNativeParameters{
				Amount:     amount,
				Bundle:     bundle,
				OriginHash: origin,
				Receiver:   receiver,
				Proof:      proof,
			},
		)
	case tokenmanager.Type_ERC20:
		return bridgeFacade.WithdrawERC20(
			opts,
			facadebind.IERC20HandlerWithdrawERC20Parameters{
				Token:      common.HexToAddress(transfer.Transfer.To.Address),
				Amount:     amount,
				Bundle:     bundle,
				OriginHash: origin,
				Receiver:   receiver,
				Proof:      proof,
				IsWrapped:  transfer.CollectionData.Wrapped,
			},
		)
	case tokenmanager.Type_ERC721:
		tokenID, err := parseTokenID(transfer.Transfer.To.TokenID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse the tokenID")
		}

		return bridgeFacade.WithdrawERC721(
			opts,
			facadebind.IERC721HandlerWithdrawERC721Parameters{
				Token:      common.HexToAddress(transfer.Transfer.To.Address),
				TokenId:    tokenID,
				TokenURI:   transfer.Item.Meta.Uri,
				Bundle:     bundle,
				OriginHash: origin,
				Receiver:   receiver,
				Proof:      proof,
				IsWrapped:  transfer.CollectionData.Wrapped,
			})
	case tokenmanager.Type_ERC1155:
		tokenID, err := parseTokenID(transfer.Transfer.To.TokenID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse the tokenID")
		}

		return bridgeFacade.WithdrawERC1155(
			opts,
			facadebind.IERC1155HandlerWithdrawERC1155Parameters{
				Token:      common.HexToAddress(transfer.Transfer.To.Address),
				TokenId:    tokenID,
				TokenURI:   transfer.Item.Meta.Uri,
				Amount:     amount,
				Bundle:     bundle,
				OriginHash: origin,
				Receiver:   receiver,
				Proof:      proof,
				IsWrapped:  transfer.CollectionData.Wrapped,
			})
	default:
		return nil, errors.Errorf("token type %d is not supported", transfer.CollectionData.TokenType)
	}
}

func (b *evmBridger) Withdraw(
	ctx context.Context,
	transfer core.TransferDetails,
) error {
	log := b.log.WithField("op_id", transfer.Origin)

	targetChain := b.mustGetChain(transfer.Transfer.To.Chain)

	withdrawn, err := b.isAlreadyWithdrawn(ctx, targetChain, transfer)
	if err != nil {
		return errors.Wrap(err, "failed to check if the transfer was already withdrawn")
	}
	if withdrawn {
		return bridge.ErrAlreadyWithdrawn
	}

	tx, err := b.makeWithdrawTx(ctx, targetChain, transfer, false)
	if err != nil {
		return errors.Wrap(err, "failed to call the withdraw method")
	}

	log.WithField("tx_id", tx.Hash()).Info("submitted transaction")

	receipt, err := bind.WaitMined(ctx, targetChain.RPC, tx)
	if err != nil {
		return errors.Wrap(err, "failed to wait for the transaction to be mined")
	}
	if receipt.Status == 0 {
		log.WithField("receipt", utils.Prettify(receipt)).Errorf("%s transaction failed", transfer.Transfer.To.Chain)
		return errors.New("transaction failed")
	}

	log.
		WithFields(logan.F{
			"tx_id":        tx.Hash(),
			"tx_index":     receipt.TransactionIndex,
			"block_number": receipt.BlockNumber,
			"gas_used":     receipt.GasUsed,
		}).
		Info("evm transaction confirmed")

	return nil
}

func getBundleData(transfer rarimocore.Transfer) (facadebind.IBundlerBundle, error) {
	if len(transfer.BundleData) == 0 {
		return facadebind.IBundlerBundle{}, nil
	}

	result := facadebind.IBundlerBundle{}
	bundle, err := hexutil.Decode(transfer.BundleData)
	if err != nil {
		return result, errors.Wrap(err, "failed to parse bundle data")
	}
	salt, err := hexutil.Decode(transfer.BundleSalt)
	if err != nil {
		return result, errors.Wrap(err, "failed to parse bundle salt")
	}

	result.Bundle = bundle
	result.Salt = utils.ToByte32(salt)

	return result, nil
}

func parseTokenID(rawTokenID string) (*big.Int, error) {
	rawBytes, err := hexutil.Decode(rawTokenID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse the tokenID: %s", logan.Field("token_id", rawTokenID))
	}

	return big.NewInt(0).SetBytes(rawBytes), nil
}

func (b *evmBridger) mustGetChain(chainName string) *config.EVMChain {
	chain, ok := b.evm.GetChainByName(chainName)
	if !ok {
		panic(errors.Errorf("unknown EVM chain: %s", chainName))
	}

	return chain
}

func (b *evmBridger) isAlreadyWithdrawn(
	ctx context.Context,
	chain *config.EVMChain,
	transfer core.TransferDetails,
) (bool, error) {
	bridger, err := bridgebind.NewBridge(chain.BridgeAddress, chain.RPC)
	if err != nil {
		return false, errors.Wrap(err, "failed to make an instance of ethereum bridger")
	}

	withdrawn, err := bridger.BridgeCaller.UsedHashes(
		&bind.CallOpts{Pending: false, Context: ctx},
		utils.ToByte32(hexutil.MustDecode(transfer.Origin)),
	)
	if err != nil {
		return false, errors.Wrap(err, "failed to check if the transfer was already withdrawn")
	}

	return withdrawn, nil
}

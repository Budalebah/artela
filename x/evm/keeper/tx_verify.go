package keeper

import (
	"errors"
	"github.com/artela-network/artela/ethereum/utils"
	"github.com/ethereum/go-ethereum/params"
	"math/big"

	errorsmod "cosmossdk.io/errors"
	"github.com/artela-network/artela/x/evm/states"
	"github.com/artela-network/aspect-core/djpm"
	cosmos "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ethereum/go-ethereum/common"
	ethereum "github.com/ethereum/go-ethereum/core/types"
)

func (k *Keeper) VerifySig(ctx cosmos.Context, tx *ethereum.Transaction) (common.Address, []byte, error) {
	// tx without signature
	txConfig := k.TxConfig(ctx, tx.Hash(), tx.Type())
	stateDB := states.New(ctx, k, txConfig)

	// calling a contract without signature, verify with aspect
	// this verification method is only allowed in call contract,
	// transactions that transfer value or creating contract must be signed
	if k.isCustomizedVerification(tx) && (stateDB.GetCodeHash(*tx.To()) != common.Hash{}) {
		return k.tryAspectVerifier(ctx, tx)
	}

	// tx with valid ec sig
	chainID := k.ChainID()
	evmParams := k.GetParams(ctx)
	chainCfg := evmParams.GetChainConfig()
	ethCfg := chainCfg.EthereumConfig(chainID)
	blockNum := big.NewInt(ctx.BlockHeight())
	signer := ethereum.MakeSigner(ethCfg, blockNum, uint64(ctx.BlockTime().Unix()))

	allowUnprotectedTxs := evmParams.GetAllowUnprotectedTxs()
	if !allowUnprotectedTxs && !tx.Protected() {
		return common.Address{}, nil, errorsmod.Wrapf(
			errortypes.ErrNotSupported,
			"rejected unprotected Ethereum transaction. Please EIP155 sign your transaction to protect it against replay-attacks")
	}
	sender, err := signer.Sender(tx)
	if err != nil {
		return common.Address{}, nil, errorsmod.Wrapf(
			errortypes.ErrorInvalidSigner,
			"couldn't retrieve sender address from the ethereum transaction: %s",
			err.Error(),
		)
	}

	return sender, nil, nil
}

func (k *Keeper) tryAspectVerifier(ctx cosmos.Context, tx *ethereum.Transaction) (common.Address, []byte, error) {
	return djpm.AspectInstance().GetSenderAndCallData(ctx.BlockHeight(), tx)
}

func (k *Keeper) MakeSigner(ctx cosmos.Context, tx *ethereum.Transaction, config *params.ChainConfig, blockNumber *big.Int, blockTime uint64) ethereum.Signer {
	txConfig := k.TxConfig(ctx, tx.Hash(), tx.Type())
	stateDB := states.New(ctx, k, txConfig)
	if k.isCustomizedVerification(tx) && (stateDB.GetCodeHash(*tx.To()) != common.Hash{}) {
		return &aspectSigner{k, ctx}
	}

	return ethereum.MakeSigner(config, blockNumber, blockTime)
}

func (k *Keeper) isCustomizedVerification(tx *ethereum.Transaction) bool {
	return utils.IsCustomizedVerification(tx)
}

func (k *Keeper) processMsgData(tx *ethereum.Transaction) ([]byte, error) {
	if k.isCustomizedVerification(tx) {
		_, callData, err := djpm.DecodeValidationAndCallData(tx.Data())
		return callData, err
	}

	return tx.Data(), nil
}

type aspectSigner struct {
	keeper *Keeper
	ctx    cosmos.Context
}

func (a *aspectSigner) Sender(tx *ethereum.Transaction) (common.Address, error) {
	sender, _, err := a.keeper.VerifySig(a.ctx, tx)
	return sender, err
}

func (a *aspectSigner) SignatureValues(tx *ethereum.Transaction, sig []byte) (r, s, v *big.Int, err error) {
	return nil, nil, nil, errors.New("not supported")
}

func (a *aspectSigner) ChainID() *big.Int {
	return a.keeper.ChainID()
}

func (a *aspectSigner) Hash(tx *ethereum.Transaction) common.Hash {
	return tx.Hash()
}

func (a *aspectSigner) Equal(signer ethereum.Signer) bool {
	_, ok := signer.(*aspectSigner)
	return ok
}
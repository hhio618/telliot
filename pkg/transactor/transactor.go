// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package transactor

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	tellorCommon "github.com/tellor-io/telliot/pkg/common"
	"github.com/tellor-io/telliot/pkg/config"
	"github.com/tellor-io/telliot/pkg/contracts"
	"github.com/tellor-io/telliot/pkg/contracts/tellor"
	"github.com/tellor-io/telliot/pkg/db"
	"github.com/tellor-io/telliot/pkg/logging"
	"github.com/tellor-io/telliot/pkg/util"
)

const ComponentName = "transactor"

// Transactor implements the Transactor interface.
type Transactor struct {
	logger           log.Logger
	cfg              *config.Config
	proxy            db.DataServerProxy
	client           contracts.ETHClient
	account          *config.Account
	contractInstance *contracts.ITellor
	nonce            string
	reqVals          [5]*big.Int
	reqIds           [5]*big.Int
	ctx              context.Context
}

func NewTransactor(logger log.Logger, cfg *config.Config, proxy db.DataServerProxy,
	client contracts.ETHClient, account *config.Account, contractInstance *contracts.ITellor) (*Transactor, error) {
	filterLog, err := logging.ApplyFilter(*cfg, ComponentName, logger)
	if err != nil {
		return nil, errors.Wrap(err, "apply filter logger")
	}
	return &Transactor{
		logger:           log.With(filterLog, "component", ComponentName, "pubKey", account.Address.String()[:6]),
		cfg:              cfg,
		proxy:            proxy,
		client:           client,
		account:          account,
		contractInstance: contractInstance,
	}, nil
}

func (t *Transactor) Transact(ctx context.Context, nonce string, reqIds [5]*big.Int, reqVals [5]*big.Int) (*types.Transaction, *types.Receipt, error) {
	t.nonce = nonce
	t.reqIds = reqIds
	t.reqVals = reqVals
	t.ctx = ctx
	gasLimit, err := t.EstimateGas()
	if err != nil {
		level.Error(t.logger).Log("msg", "getting gasLimit", "err", err)
	}
	slotNum, err := t.contractInstance.GetUintVar(nil, util.Keccak256([]byte("_SLOT_PROGRESS")))
	if err != nil {
		level.Error(t.logger).Log("msg", "getting _SLOT_PROGRESS", "err", err)
	} else {
		level.Info(t.logger).Log("msg", "*********** slot number", "slotNum", slotNum)
	}
	tx, err := SubmitContractTxn(ctx, t.logger, t.cfg, t.proxy, t.client, t.contractInstance, t.account, "submitSolution", t.submit)
	if err != nil {
		return nil, nil, errors.Wrap(err, "submitting the transaction")
	}
	receipt, err := bind.WaitMined(context.Background(), t.client, tx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "transaction result")
	}
	if receipt.Status != 1 {
		return nil, nil, errors.New("unsuccessful transaction status")
	}
	gasUsed := big.NewInt(int64(receipt.GasUsed))
	refund := gasLimit - gasUsed.Uint64()
	if slotNum.Uint64() == 0 || gasUsed.Uint64() > 1000000 {
		level.Info(t.logger).Log("msg", "*********** gas usage", "gasLimit", gasLimit, "gasUsed", gasUsed, "gasRefund", refund, "txHash", tx.Hash().String())
	}
	return tx, receipt, nil
}

func (t *Transactor) submit(ctx context.Context, options *bind.TransactOpts) (*types.Transaction, error) {

	txn, err := t.contractInstance.SubmitMiningSolution(options,
		t.nonce,
		t.reqIds,
		t.reqVals)
	if err != nil {
		return nil, err
	}
	return txn, err
}

func SubmitContractTxn(
	ctx context.Context,
	logger log.Logger,
	cfg *config.Config,
	proxy db.DataServerProxy,
	client contracts.ETHClient,
	tellor *contracts.ITellor,
	account *config.Account,
	ctxName string,
	callback tellorCommon.TransactionGeneratorFN,
) (*types.Transaction, error) {

	nonce, err := client.NonceAt(ctx, account.Address)
	if err != nil {
		return nil, errors.Wrap(err, "getting nonce for miner address")
	}

	// Use the same nonce in case there is a stuck transaction so that it submits with the current nonce but higher gas price.
	IntNonce := int64(nonce)
	keys := []string{
		db.GasKey,
	}
	m, err := proxy.BatchGet(keys)
	if err != nil {
		return nil, errors.Wrap(err, "getting data from the db")
	}
	gasPrice := getInt(m[db.GasKey])
	if gasPrice.Cmp(big.NewInt(0)) == 0 {
		level.Warn(logger).Log("msg", "no gas price from DB, falling back to client suggested gas price")
		gasPrice, err = client.SuggestGasPrice(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "determine gas price to submit txn")
		}
	}
	mul := cfg.GasMultiplier
	if mul > 0 {
		level.Info(logger).Log("msg", "settings gas price multiplier", "value", mul)
		gasPrice = gasPrice.Mul(gasPrice, big.NewInt(int64(mul)))
	}

	var finalError error
	for i := 0; i <= 5; i++ {
		balance, err := client.BalanceAt(ctx, account.Address, nil)
		if err != nil {
			finalError = err
			continue
		}

		cost := big.NewInt(1)
		cost = cost.Mul(gasPrice, big.NewInt(200000))
		if balance.Cmp(cost) < 0 {
			// FIXME: notify someone that we're out of funds!
			finalError = errors.Errorf("insufficient funds to send transaction: %v < %v", balance, cost)
			continue
		}

		netID, err := client.NetworkID(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "getting network id")
		}
		auth, err := bind.NewKeyedTransactorWithChainID(account.PrivateKey, netID)
		if err != nil {
			return nil, errors.Wrap(err, "creating transactor")
		}
		auth.Nonce = big.NewInt(IntNonce)
		auth.Value = big.NewInt(0)      // in weiF
		auth.GasLimit = uint64(3000000) // in units
		if gasPrice.Cmp(big.NewInt(0)) == 0 {
			gasPrice = big.NewInt(100)
		}
		if i > 1 {
			gasPrice1 := new(big.Int).Set(gasPrice)
			gasPrice1.Mul(gasPrice1, big.NewInt(int64(i*11))).Div(gasPrice1, big.NewInt(int64(100)))
			auth.GasPrice = gasPrice1.Add(gasPrice, gasPrice1)
		} else {
			// First time, try base gas price.
			auth.GasPrice = gasPrice
		}
		max := cfg.GasMax
		var maxGasPrice *big.Int
		gasPrice1 := big.NewInt(tellorCommon.GWEI)
		if max > 0 {
			maxGasPrice = gasPrice1.Mul(gasPrice1, big.NewInt(int64(max)))
		} else {
			maxGasPrice = gasPrice1.Mul(gasPrice1, big.NewInt(int64(100)))
		}

		if auth.GasPrice.Cmp(maxGasPrice) > 0 {
			level.Info(logger).Log("msg", "gas price too high, will default to the max price", "current", auth.GasPrice, "defaultMax", maxGasPrice)
			auth.GasPrice = maxGasPrice
		}

		tx, err := callback(ctx, auth)
		if err != nil {
			if strings.Contains(err.Error(), "nonce too low") { // Can't use error type matching because of the way the eth client is implemented.
				IntNonce = IntNonce + 1
				level.Debug(logger).Log("msg", "last transaction has been confirmed so will increase the nonce and resend the transaction.")

			} else if strings.Contains(err.Error(), "replacement transaction underpriced") { // Can't use error type matching because of the way the eth client is implemented.
				level.Debug(logger).Log("msg", "last transaction is stuck so will increase the gas price and try to resend")
				finalError = err
			} else {
				finalError = errors.Wrap(err, "callback")
			}

			delay := 15 * time.Second
			level.Debug(logger).Log("msg", "will retry a send", "retryDelay", delay)
			select {
			case <-ctx.Done():
				return nil, errors.New("the submit context was canceled")
			case <-time.After(delay):
				continue
			}
		}
		return tx, nil
	}
	return nil, errors.Wrapf(finalError, "submit txn after 5 attempts ctx:%v", ctxName)
}
func (t *Transactor) EstimateGas() (uint64, error) {
	gasPrice, err := t.client.SuggestGasPrice(context.Background())
	if err != nil {
		return 0, err
	}
	abiP, err := abi.JSON(strings.NewReader(tellor.TellorABI))
	if err != nil {
		return 0, err
	}

	packed, err := abiP.Pack("submitMiningSolution", t.nonce, t.reqIds, t.reqVals)
	if err != nil {
		return 0, err
	}
	x := common.HexToAddress(config.TellorAddress)
	data := ethereum.CallMsg{
		From:     t.account.Address,
		To:       &x,
		GasPrice: gasPrice,
		Data:     packed,
	}

	gasLimit, err := t.client.EstimateGas(t.ctx, data)
	if err != nil {
		return 0, fmt.Errorf("failed to estimate gas needed: %v", err)
	}
	return gasLimit, err
}

func getInt(data []byte) *big.Int {
	if len(data) == 0 {
		return nil
	}

	val, err := hexutil.DecodeBig(string(data))
	if err != nil {
		return nil
	}
	return val
}

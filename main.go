package main

import (
	"context"
	"log"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/tellor-io/telliot/pkg/contracts/tellor"
)

type RefundChecker struct {
	client *ethclient.Client
}

func main() {
	client, err := ethclient.Dial(os.Getenv("NODE_URL"))
	if err != nil {
		log.Fatal(err)
	}

	self := &RefundChecker{
		client: client,
	}
	self.monitorRefund()
}

func (self *RefundChecker) monitorRefund() {
	var err error

	iter, err := self.TellorNonceIterator()
	if err != nil {
		log.Fatalf("getting TellorNonceIterator: %v", err)
	}
	for iter.Next() {
		self.printRefund(context.Background(), iter.Event)
	}
}

func (self *RefundChecker) printRefund(ctx context.Context, event *tellor.TellorNonceSubmitted) {
	for {
		select {
		case <-ctx.Done():
			log.Print("msg", "transaction confirmation check canceled")
			return
		default:
		}
		receipt, err := self.client.TransactionReceipt(ctx, event.Raw.TxHash)
		if err != nil {
			log.Print("msg", "receipt retrieval", "err", err)
		} else if receipt != nil && receipt.Status == types.ReceiptStatusSuccessful { // Failed transactions cost is monitored in a different process.
			tx, _, err := self.client.TransactionByHash(ctx, event.Raw.TxHash)
			if err != nil {
				log.Print("msg", "get transaction by hash", "err", err)
				return
			}
			refund, _ := big.NewFloat(0).Sub(big.NewFloat(float64(tx.Gas())), big.NewFloat(float64(receipt.GasUsed))).Float64()
			log.Printf("REFUND=%v, tx=%v, slot=%v", refund, tx.Hash().String(), event.Slot)
			return
		}

		log.Print("msg", "transaction not yet mined")
		continue
	}
}

func (self *RefundChecker) TellorNonceIterator() (*tellor.TellorNonceSubmittedIterator, error) {
	var tellorFilterer *tellor.TellorFilterer
	tellorFilterer, err := tellor.NewTellorFilterer(common.HexToAddress("0x88dF592F8eb5D7Bd38bFeF7dEb0fBc02cf3778a0"), self.client)
	if err != nil {
		return nil, errors.Wrap(err, "getting ITellorFilterer instance")
	}
	iter, err := tellorFilterer.FilterNonceSubmitted(
		&bind.FilterOpts{
			Start: 12689004,
			End:   nil,
		},
		nil,
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "getting  channel")
	}
	return iter, nil
}

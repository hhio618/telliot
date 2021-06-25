package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
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
		fmt.Printf("iter: %v", iter)
	}
}

func (self *RefundChecker) TellorNonceIterator() (*tellor.ITellorNonceSubmittedIterator, error) {
	var tellorFilterer *tellor.ITellorFilterer
	tellorFilterer, err := tellor.NewITellorFilterer(common.HexToAddress("0x88dF592F8eb5D7Bd38bFeF7dEb0fBc02cf3778a0"), self.client)
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

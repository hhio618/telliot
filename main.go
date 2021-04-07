package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/event"
	"github.com/pkg/errors"
	"github.com/tellor-io/telliot/pkg/contracts/tellor"
)

func main() {
	client, err := ethclient.Dial(os.Getenv("NODE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	contract, err := tellor.NewITellor(tellorAddress, client)
	if err != nil {
		log.Fatal(err)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var sub event.Subscription
	events := make(chan *tellor.TellorNonceSubmitted)

	for {
		sub, err = subscribe(client, events)
		if err != nil {
			log.Print("msg", "initial subscribing to  events failed")
			<-ticker.C
			continue
		}
		break
	}
	var ctx context.Context
	var lastCncl context.CancelFunc
	for {
		select {
		case err := <-sub.Err():
			if err != nil {
				log.Print(
					"msg",
					" subscription error",
					"err", err)
			}
			// Trying to resubscribe until it succeeds.
			for {
				sub, err = subscribe(client, events)
				if err != nil {
					log.Print("msg", "re-subscribing to  events failed")
					<-ticker.C
					continue
				}
				break
			}
			log.Print("msg", "re-subscribed to  events")
		case event := <-events:
			if lastCncl != nil {
				lastCncl()
			}
			if event.Slot.Uint64() == 3 {
				ctx, lastCncl = context.WithCancel(context.Background())
				go estimate(ctx, contract, client)
			}
		}
	}
}

func subscribe(client *ethclient.Client, output chan *tellor.TellorNonceSubmitted) (event.Subscription, error) {
	var tellorFilterer *tellor.TellorFilterer
	tellorFilterer, err := tellor.NewTellorFilterer(common.HexToAddress("0x88dF592F8eb5D7Bd38bFeF7dEb0fBc02cf3778a0"), client)
	if err != nil {
		return nil, errors.Wrap(err, "getting ITellorFilterer instance")
	}
	sub, err := tellorFilterer.WatchNonceSubmitted(
		nil,
		output,
		nil,
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "getting  channel")
	}
	return sub, nil
}

var (
	tellorAddress = common.HexToAddress("0x88df592f8eb5d7bd38bfef7deb0fbc02cf3778a0")
)

func estimate(ctx context.Context, contract *tellor.ITellor, client *ethclient.Client) {
	ticker := time.NewTicker(1 * time.Second)
	for {
		gasPrice, err := client.SuggestGasPrice(ctx)
		if err != nil {
			log.Print(err)
			<-ticker.C
			continue
		}
		abiP, err := abi.JSON(strings.NewReader(tellor.TellorABI))
		if err != nil {
			log.Fatal(err)
		}
		vars, err := contract.GetNewCurrentVariables(nil)
		if err != nil {
			log.Print(err)
			<-ticker.C
			continue
		}
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		reqVals := [5]*big.Int{
			new(big.Int).SetUint64(r.Uint64()),
			new(big.Int).SetUint64(r.Uint64()),
			new(big.Int).SetUint64(r.Uint64()),
			new(big.Int).SetUint64(r.Uint64()),
			new(big.Int).SetUint64(r.Uint64()),
		}
		packed, err := abiP.Pack("submitMiningSolution", "", vars.RequestIds, reqVals)
		if err != nil {
			log.Fatal(err)
		}
		data := ethereum.CallMsg{
			From:     common.HexToAddress("0xDD6D1C35518fc955BBBeb52C5c1f5Fb4E16D7EAF"),
			To:       &tellorAddress,
			GasPrice: gasPrice,
			Data:     packed,
		}
		gasUsed, err := client.EstimateGas(ctx, data)
		if err != nil {
			log.Printf("while estimate gas: %v", err)
			<-ticker.C
			continue
		}
		fmt.Println("gasUsed", gasUsed)
		break
	}

}

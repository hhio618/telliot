// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package dispute

import (
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/tellor-io/telliot/pkg/config"
	"github.com/tellor-io/telliot/pkg/contracts"
	"github.com/tellor-io/telliot/pkg/tracker/index"
	"github.com/tellor-io/telliot/pkg/util"
)

const ComponentName = "dispute"

type Config struct {
	DisputeTimeDelta util.Duration // Ignore data further than this away from the value we are checking.
	DisputeThreshold float64       // Maximum allowed relative difference between observed and submitted value.
}

// type disputeChecker struct {
// 	config           *Config
// 	client           contracts.ETHClient
// 	contract         *contracts.ITellor
// 	lastCheckedBlock uint64
// 	logger           log.Logger
// }

// func (c *disputeChecker) String() string {
// 	return "DisputeChecker"
// }

// // ValueCheckResult holds the details regarding the disputed value.
// type ValueCheckResult struct {
// 	High, Low   float64
// 	WithinRange bool
// 	Datapoints  []float64
// 	Times       []time.Time
// }

// // CheckValueAtTime queries for the details regarding the disputed value.
// func CheckValueAtTime(cfg *config.Config, reqID uint64, val *big.Int, at time.Time) (*ValueCheckResult, error) {

// 	// Check the value in 5 places, spread over cfg.Trackers.DisputeTimeDelta.Duration.
// 	var datapoints []float64
// 	var times []time.Time
// 	for i := 0; i < 5; i++ {
// 		t := at.Add((time.Duration(i) - 2) * cfg.Trackers.DisputeTimeDelta.Duration / 5)
// 		fval, confidence, err := index.PSRValueForTime(int(reqID), t, cfg.Trackers.SleepCycle.Seconds())
// 		if err != nil {
// 			return nil, err
// 		}
// 		if confidence > 0.8 {
// 			datapoints = append(datapoints, fval)
// 			times = append(times, t)
// 		}
// 	}

// 	if len(datapoints) == 0 {
// 		return nil, nil
// 	}

// 	min := math.MaxFloat64
// 	max := 0.0

// 	for _, dp := range datapoints {
// 		if dp > max {
// 			max = dp
// 		}
// 		if dp < min {
// 			min = dp
// 		}
// 	}
// 	min *= 1 - cfg.Trackers.DisputeThreshold
// 	max *= 1 + cfg.Trackers.DisputeThreshold

// 	bigF := new(big.Float)
// 	bigF.SetInt(val)
// 	floatVal, _ := bigF.Float64()

// 	withinRange := (floatVal > min) && (floatVal < max)

// 	return &ValueCheckResult{
// 		Low:         min,
// 		High:        max,
// 		WithinRange: withinRange,
// 		Datapoints:  datapoints,
// 		Times:       times,
// 	}, nil
// }

// func NewDisputeChecker(
// 	logger log.Logger,
// 	config *config.Config,
// 	client contracts.ETHClient,
// 	contract *contracts.ITellor,
// 	lastCheckedBlock uint64,
// ) *disputeChecker {
// 	return &disputeChecker{
// 		client:           client,
// 		contract:         contract,
// 		config:           config,
// 		lastCheckedBlock: lastCheckedBlock,
// 		logger:           log.With(logger, "component", ComponentName),
// 	}
// }

// func (c *disputeChecker) Exec(ctx context.Context) error {

// 	header, err := c.client.HeaderByNumber(ctx, nil)
// 	if err != nil {
// 		return errors.Wrap(err, "get latest eth block header")
// 	}
// 	if c.lastCheckedBlock == 0 {
// 		c.lastCheckedBlock = header.Number.Uint64()
// 	}

// 	toCheck := header.Number.Uint64()

// 	const blockDelay = 100
// 	if toCheck-c.lastCheckedBlock < blockDelay {
// 		return nil
// 	}

// 	abi, err := abi.JSON(strings.NewReader(contracts.ITellorABI))
// 	if err != nil {
// 		return errors.Wrap(err, "parse abi")
// 	}

// 	//just use nil for most of the variables, only using this object to call UnpackLog which only uses the abi
// 	bar := bind.NewBoundContract(c.contract.Address, abi, nil, nil, nil)

// 	checkUntil := toCheck - blockDelay
// 	nonceSubmitID := abi.Events["NonceSubmitted"].ID
// 	query := ethereum.FilterQuery{
// 		FromBlock: big.NewInt(int64(c.lastCheckedBlock)),
// 		ToBlock:   big.NewInt(int64(checkUntil)),
// 		Addresses: []common.Address{c.contract.Address},
// 		Topics:    [][]common.Hash{{nonceSubmitID}},
// 	}
// 	logs, err := c.client.FilterLogs(ctx, query)
// 	if err != nil {
// 		return errors.Wrap(err, "filter eth logs")
// 	}
// 	blockTimes := make(map[uint64]time.Time)
// 	for _, l := range logs {
// 		nonceSubmit := contracts.TellorNonceSubmitted{}
// 		err := bar.UnpackLog(&nonceSubmit, "NonceSubmitted", l)
// 		if err != nil {
// 			return errors.Wrap(err, "unpack into object")
// 		}
// 		blockTime, ok := blockTimes[l.BlockNumber]
// 		if !ok {
// 			header, err := c.client.HeaderByNumber(ctx, big.NewInt(int64(l.BlockNumber)))
// 			if err != nil {
// 				return errors.Wrap(err, "get nonce block header")
// 			}
// 			blockTime = time.Unix(int64(header.Time), 0)
// 			blockTimes[l.BlockNumber] = blockTime
// 		}
// 		for i, reqID := range nonceSubmit.RequestId {
// 			result, err := CheckValueAtTime(c.config, nonceSubmit.RequestId[i].Uint64(), nonceSubmit.Value[i], blockTime)
// 			if err != nil {
// 				return err
// 			} else if result == nil {
// 				level.Warn(c.logger).Log("msg", "no value data", "reqid", reqID, "blockTime", blockTime)
// 				continue
// 			}

// 			if !result.WithinRange {
// 				s := fmt.Sprintf("suspected incorrect value for requestID %d at %s:\n , nearest values:\n", reqID, blockTime)
// 				for i, pt := range result.Datapoints {
// 					s += fmt.Sprintf("\t%.0f, ", pt)
// 					delta := blockTime.Sub(result.Times[i])
// 					if delta > 0 {
// 						s += fmt.Sprintf("%s before\n", delta.String())
// 					} else {
// 						s += fmt.Sprintf("%s after\n", (-delta).String())
// 					}
// 				}
// 				s += fmt.Sprintf("value submitted by miner with address %s", nonceSubmit.Miner)
// 				level.Error(c.logger).Log("msg", s)
// 				filename := fmt.Sprintf("possible-dispute-%s.txt", blockTime)
// 				err := ioutil.WriteFile(filename, []byte(s), 0655)
// 				if err != nil {
// 					level.Error(c.logger).Log("msg", "saving dispute data", "filename", filename, "err", err)
// 				}
// 			} else {
// 				level.Info(c.logger).Log("msg", "value appears to be within expected range", "reqID", reqID, "value", nonceSubmit.Value, "blockTime", blockTime.String())
// 			}

// 		}
// 	}
// 	c.lastCheckedBlock = checkUntil
// 	return nil
// }
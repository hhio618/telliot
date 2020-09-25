// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package routes

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/tellor-io/TellorMiner/common"
	"github.com/tellor-io/TellorMiner/db"
)

type TributeBalanceHandler struct {
}

func (h *TributeBalanceHandler) Incoming(ctx context.Context, req *http.Request) (int, string) {
	DB := ctx.Value(common.DBContextKey).(db.DB)
	v, err := DB.Get(db.TributeBalanceKey)
	if err != nil {
		log.Printf("Problem reading TributeBalance from DB: %v\n", err)
		return 500, `{"error": "Could not read TributeBalance from DB"}`
	}
	return 200, fmt.Sprintf(`{ "TributeBalance": "%s" }`, string(v))
}

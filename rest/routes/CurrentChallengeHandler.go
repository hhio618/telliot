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

type CurrentChallengeHandler struct {
}

func (h *CurrentChallengeHandler) Incoming(ctx context.Context, req *http.Request) (int, string) {
	DB := ctx.Value(common.DBContextKey).(db.DB)
	v, err := DB.Get(db.CurrentChallengeKey)
	if err != nil {
		log.Printf("Problem reading CurrentChallenge from DB: %v\n", err)
		return 500, `{"error": "Could not read CurrentChallenge from DB"}`
	}
	return 200, fmt.Sprintf(`{ "currentChallenge": "%s" }`, string(v))
}

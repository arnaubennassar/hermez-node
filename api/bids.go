package api

import (
	"net/http"

	"github.com/arnaubennassar/hermez-node/api/parsers"
	"github.com/arnaubennassar/hermez-node/db/historydb"
	"github.com/gin-gonic/gin"
)

func (a *API) getBids(c *gin.Context) {
	filters, err := parsers.ParseBidsFilters(c, a.validate)
	if err != nil {
		retBadReq(err, c)
		return
	}

	bids, pendingItems, err := a.h.GetBidsAPI(historydb.GetBidsAPIRequest{
		SlotNum:    filters.SlotNum,
		BidderAddr: filters.BidderAddr,
		FromItem:   filters.FromItem,
		Limit:      filters.Limit,
		Order:      filters.Order,
	})

	if err != nil {
		retSQLErr(err, c)
		return
	}

	// Build successful response
	type bidsResponse struct {
		Bids         []historydb.BidAPI `json:"bids"`
		PendingItems uint64             `json:"pendingItems"`
	}
	c.JSON(http.StatusOK, &bidsResponse{
		Bids:         bids,
		PendingItems: pendingItems,
	})
}

package api

import (
	"net/http"

	"github.com/arnaubennassar/hermez-node/api/parsers"
	"github.com/arnaubennassar/hermez-node/db/historydb"
	"github.com/gin-gonic/gin"
)

func (a *API) getExits(c *gin.Context) {
	// Get query parameters
	exitsFilters, err := parsers.ParseExitsFilters(c, a.validate)
	if err != nil {
		retBadReq(err, c)
		return
	}

	// Fetch exits from historyDB
	exits, pendingItems, err := a.h.GetExitsAPI(exitsFilters)
	if err != nil {
		retSQLErr(err, c)
		return
	}

	// Build successful response
	type exitsResponse struct {
		Exits        []historydb.ExitAPI `json:"exits"`
		PendingItems uint64              `json:"pendingItems"`
	}
	c.JSON(http.StatusOK, &exitsResponse{
		Exits:        exits,
		PendingItems: pendingItems,
	})
}

func (a *API) getExit(c *gin.Context) {
	// Get batchNum and accountIndex
	batchNum, idx, err := parsers.ParseExitFilter(c)
	if err != nil {
		retBadReq(err, c)
		return
	}
	// Fetch tx from historyDB
	exit, err := a.h.GetExitAPI(batchNum, idx)
	if err != nil {
		retSQLErr(err, c)
		return
	}
	// Build successful response
	c.JSON(http.StatusOK, exit)
}

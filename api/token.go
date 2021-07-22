package api

import (
	"net/http"

	"github.com/arnaubennassar/hermez-node/api/parsers"
	"github.com/arnaubennassar/hermez-node/common"
	"github.com/arnaubennassar/hermez-node/db/historydb"
	"github.com/gin-gonic/gin"
)

func (a *API) getToken(c *gin.Context) {
	// Get TokenID
	tokenIDUint, err := parsers.ParseTokenFilter(c)
	if err != nil {
		retBadReq(err, c)
		return
	}
	tokenID := common.TokenID(*tokenIDUint)
	// Fetch token from historyDB
	token, err := a.h.GetTokenAPI(tokenID)
	if err != nil {
		retSQLErr(err, c)
		return
	}
	c.JSON(http.StatusOK, token)
}

func (a *API) getTokens(c *gin.Context) {
	// Account filters
	filters, err := parsers.ParseTokensFilters(c)
	if err != nil {
		retBadReq(err, c)
		return
	}
	// Fetch exits from historyDB
	tokens, pendingItems, err := a.h.GetTokensAPI(filters)
	if err != nil {
		retSQLErr(err, c)
		return
	}

	// Build successful response
	type tokensResponse struct {
		Tokens       []historydb.TokenWithUSD `json:"tokens"`
		PendingItems uint64                   `json:"pendingItems"`
	}
	c.JSON(http.StatusOK, &tokensResponse{
		Tokens:       tokens,
		PendingItems: pendingItems,
	})
}

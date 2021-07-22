package api

import (
	"errors"
	"net/http"

	"github.com/arnaubennassar/hermez-node/api/parsers"
	"github.com/arnaubennassar/hermez-node/db/historydb"
	"github.com/gin-gonic/gin"
)

func (a *API) getFiatCurrency(c *gin.Context) {
	// Get symbol
	symbol, err := parsers.ParseCurrencyFilter(c)
	if err != nil {
		retBadReq(errors.New(ErrInvalidSymbol), c)
		return
	}
	// Fetch currency from historyDB
	currency, err := a.h.GetCurrencyAPI(symbol)
	if err != nil {
		retSQLErr(err, c)
		return
	}
	c.JSON(http.StatusOK, currency)
}

// CurrenciesResponse is the response object for multiple fiat prices
type CurrenciesResponse struct {
	Currencies []historydb.FiatCurrency `json:"currencies"`
}

func (a *API) getFiatCurrencies(c *gin.Context) {
	// Currency filters
	symbols, err := parsers.ParseCurrenciesFilters(c)
	if err != nil {
		retBadReq(err, c)
		return
	}

	// Fetch exits from historyDB
	currencies, err := a.h.GetCurrenciesAPI(symbols)
	if err != nil {
		retSQLErr(err, c)
		return
	}

	// Build successful response
	c.JSON(http.StatusOK, &CurrenciesResponse{
		Currencies: currencies,
	})
}

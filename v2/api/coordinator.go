package api

import (
	"net/http"

	"github.com/arnaubennassar/hermez-node/v2/api/parsers"
	"github.com/arnaubennassar/hermez-node/v2/db/historydb"
	"github.com/gin-gonic/gin"
)

func (a *API) getCoordinators(c *gin.Context) {
	filters, err := parsers.ParseCoordinatorsFilters(c)
	if err != nil {
		retBadReq(&apiError{
			Err:  err,
			Code: ErrParamValidationFailedCode,
			Type: ErrParamValidationFailedType,
		}, c)
		return
	}

	// Fetch coordinators from historyDB
	coordinators, pendingItems, err := a.h.GetCoordinatorsAPI(filters)
	if err != nil {
		retSQLErr(err, c)
		return
	}

	// Build successful response
	type coordinatorsResponse struct {
		Coordinators []historydb.CoordinatorAPI `json:"coordinators"`
		PendingItems uint64                     `json:"pendingItems"`
	}
	c.JSON(http.StatusOK, &coordinatorsResponse{
		Coordinators: coordinators,
		PendingItems: pendingItems,
	})
}

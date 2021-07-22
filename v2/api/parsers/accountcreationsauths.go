package parsers

import (
	"github.com/arnaubennassar/hermez-node/v2/common"
	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"
	"github.com/hermeznetwork/tracerr"
)

// GetAccountCreationAuthFilter struct for parsing hezEthereumAddress from /account-creation-authorization/:hezEthereumAddress request
type GetAccountCreationAuthFilter struct {
	Addr string `uri:"hezEthereumAddress" binding:"required"`
}

// ParseGetAccountCreationAuthFilter parsing uri request to the eth address
func ParseGetAccountCreationAuthFilter(c *gin.Context) (*ethCommon.Address, error) {
	var getAccountCreationAuthFilter GetAccountCreationAuthFilter
	if err := c.ShouldBindUri(&getAccountCreationAuthFilter); err != nil {
		return nil, tracerr.Wrap(err)
	}
	return common.HezStringToEthAddr(getAccountCreationAuthFilter.Addr, "hezEthereumAddress")
}

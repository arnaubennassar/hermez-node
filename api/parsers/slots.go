package parsers

import (
	"github.com/arnaubennassar/hermez-node/common"
	"github.com/arnaubennassar/hermez-node/db/historydb"
	"github.com/gin-gonic/gin"
	"github.com/hermeznetwork/tracerr"
)

// SlotFilter struct to get slot filter uri param from /slots/:slotNum request
type SlotFilter struct {
	SlotNum *uint `uri:"slotNum" binding:"required"`
}

// ParseSlotFilter func to parse slot filter from uri to the slot number
func ParseSlotFilter(c *gin.Context) (*uint, error) {
	var slotFilter SlotFilter
	if err := c.ShouldBindUri(&slotFilter); err != nil {
		return nil, tracerr.Wrap(err)
	}
	return slotFilter.SlotNum, nil
}

// SlotsFilters struct to get slots filters from query params from /slots request
type SlotsFilters struct {
	MinSlotNum           *int64 `form:"minSlotNum" binding:"omitempty,min=0"`
	MaxSlotNum           *int64 `form:"maxSlotNum" binding:"omitempty,min=0"`
	WonByEthereumAddress string `form:"wonByEthereumAddress"`
	FinishedAuction      *bool  `form:"finishedAuction"`

	Pagination
}

// ParseSlotsFilters func for parsing slots filters to the GetBestBidsAPIRequest
func ParseSlotsFilters(c *gin.Context) (historydb.GetBestBidsAPIRequest, error) {
	var slotsFilters SlotsFilters
	if err := c.ShouldBindQuery(&slotsFilters); err != nil {
		return historydb.GetBestBidsAPIRequest{}, err
	}

	wonByEthereumAddress, err := common.StringToEthAddr(slotsFilters.WonByEthereumAddress)
	if err != nil {
		return historydb.GetBestBidsAPIRequest{}, tracerr.Wrap(err)
	}

	return historydb.GetBestBidsAPIRequest{
		MinSlotNum:      slotsFilters.MinSlotNum,
		MaxSlotNum:      slotsFilters.MaxSlotNum,
		BidderAddr:      wonByEthereumAddress,
		FinishedAuction: slotsFilters.FinishedAuction,
		FromItem:        slotsFilters.FromItem,
		Order:           *slotsFilters.Order,
		Limit:           slotsFilters.Limit,
	}, nil
}

package api

import (
	"net/http"
	"time"

	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"
	"github.com/hermeznetwork/hermez-node/common"
	"github.com/hermeznetwork/hermez-node/db/historydb"
)

// Network define status of the network
type Network struct {
	LastBlock   int64              `json:"lastBlock"`
	LastBatch   historydb.BatchAPI `json:"lastBatch"`
	CurrentSlot int64              `json:"currentSlot"`
	NextForgers []NextForger       `json:"nextForgers"`
}

// NextForger  is a representation of the information of a coordinator and the period will forge
type NextForger struct {
	Coordinator historydb.CoordinatorAPI
	Period      Period
}

// Period is a representation of a period
type Period struct {
	SlotNum       int64
	FromBlock     int64
	ToBlock       int64
	FromTimestamp time.Time
	ToTimestamp   time.Time
}

var bootCoordinator historydb.CoordinatorAPI = historydb.CoordinatorAPI{
	ItemID: 0,
	Bidder: ethCommon.HexToAddress("0x111111111111111111111111111111111111111"),
	Forger: ethCommon.HexToAddress("0x111111111111111111111111111111111111111"),
	URL:    "https://bootCoordinator",
}

func (a *API) getState(c *gin.Context) {
	c.JSON(http.StatusOK, a.status)
}

// SC Vars

// SetRollupVariables set Status.Rollup variables
func (a *API) SetRollupVariables(rollupVariables common.RollupVariables) {
	a.status.Rollup = rollupVariables
}

// SetWDelayerVariables set Status.WithdrawalDelayer variables
func (a *API) SetWDelayerVariables(wDelayerVariables common.WDelayerVariables) {
	a.status.WithdrawalDelayer = wDelayerVariables
}

// SetAuctionVariables set Status.Auction variables
func (a *API) SetAuctionVariables(auctionVariables common.AuctionVariables) {
	a.status.Auction = auctionVariables
}

// Network

// UpdateNetworkInfo update Status.Network information
func (a *API) UpdateNetworkInfo(lastBlock common.Block, lastBatchNum common.BatchNum, currentSlot int64) error {
	a.status.Network.LastBlock = lastBlock.EthBlockNum
	lastBatch, err := a.h.GetBatchAPI(lastBatchNum)
	if err != nil {
		return err
	}
	a.status.Network.LastBatch = *lastBatch
	a.status.Network.CurrentSlot = currentSlot
	lastClosedSlot := currentSlot + int64(a.status.Auction.ClosedAuctionSlots)
	nextForgers, err := a.GetNextForgers(lastBlock, currentSlot, lastClosedSlot)
	if err != nil {
		return err
	}
	a.status.Network.NextForgers = nextForgers
	return nil
}

// GetNextForgers returns next forgers
func (a *API) GetNextForgers(lastBlock common.Block, currentSlot, lastClosedSlot int64) ([]NextForger, error) {
	secondsPerBlock := int64(15) //nolint:gomnd
	// currentSlot and lastClosedSlot included
	limit := uint(lastClosedSlot - currentSlot + 1)
	bids, _, err := a.h.GetBestBidsAPI(&currentSlot, &lastClosedSlot, nil, &limit, "ASC")
	if err != nil {
		return nil, err
	}
	nextForgers := []NextForger{}
	// Create nextForger for each slot
	for i := currentSlot; i <= lastClosedSlot; i++ {
		fromBlock := i*int64(a.cg.AuctionConstants.BlocksPerSlot) + a.cg.AuctionConstants.GenesisBlockNum
		toBlock := (i+1)*int64(a.cg.AuctionConstants.BlocksPerSlot) + a.cg.AuctionConstants.GenesisBlockNum - 1
		nextForger := NextForger{
			Period: Period{
				SlotNum:       i,
				FromBlock:     fromBlock,
				ToBlock:       toBlock,
				FromTimestamp: lastBlock.Timestamp.Add(time.Second * time.Duration(secondsPerBlock*(fromBlock-lastBlock.EthBlockNum))),
				ToTimestamp:   lastBlock.Timestamp.Add(time.Second * time.Duration(secondsPerBlock*(toBlock-lastBlock.EthBlockNum))),
			},
		}
		foundBid := false
		// If there is a bid for a slot, get forger (coordinator)
		for j := range bids {
			if bids[j].SlotNum == i {
				foundBid = true
				coordinator, err := a.h.GetCoordinatorAPI(bids[j].Bidder)
				if err != nil {
					return nil, err
				}
				nextForger.Coordinator = *coordinator
				break
			}
		}
		// If there is no bid, the coordinator that will forge is boot coordinator
		if !foundBid {
			nextForger.Coordinator = bootCoordinator
		}
		nextForgers = append(nextForgers, nextForger)
	}
	return nextForgers, nil
}

// Metrics

// UpdateMetrics update Status.Metrics information
func (a *API) UpdateMetrics() error {
	metrics, err := a.h.GetMetrics(a.status.Network.LastBatch.BatchNum)
	if err != nil {
		return err
	}
	a.status.Metrics = *metrics
	return nil
}

// Recommended fee

// UpdateRecommendedFee update Status.RecommendedFee information
func (a *API) UpdateRecommendedFee() error {
	return nil
}
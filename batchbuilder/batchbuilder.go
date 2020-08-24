package batchbuilder

import (
	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/hermeznetwork/hermez-node/common"
	"github.com/hermeznetwork/hermez-node/db/statedb"
)

// ConfigCircuit contains the circuit configuration
type ConfigCircuit struct {
	TxsMax       uint64
	L1TxsMax     uint64
	SMTLevelsMax uint64
}

// BatchBuilder implements the batch builder type, which contains the
// functionalities
type BatchBuilder struct {
	localStateDB   *statedb.LocalStateDB
	configCircuits []ConfigCircuit
}

// ConfigBatch contains the batch configuration
type ConfigBatch struct {
	ForgerAddress ethCommon.Address
}

// NewBatchBuilder constructs a new BatchBuilder, and executes the bb.Reset
// method
func NewBatchBuilder(dbpath string, synchronizerStateDB *statedb.StateDB, configCircuits []ConfigCircuit, batchNum uint64, nLevels uint64) (*BatchBuilder, error) {
	localStateDB, err := statedb.NewLocalStateDB(dbpath, synchronizerStateDB, true, int(nLevels))
	if err != nil {
		return nil, err
	}

	bb := BatchBuilder{
		localStateDB:   localStateDB,
		configCircuits: configCircuits,
	}

	err = bb.Reset(batchNum, true)
	return &bb, err
}

// Reset tells the BatchBuilder to reset it's internal state to the required
// `batchNum`.  If `fromSynchronizer` is true, the BatchBuilder must take a
// copy of the rollup state from the Synchronizer at that `batchNum`, otherwise
// it can just roll back the internal copy.
func (bb *BatchBuilder) Reset(batchNum uint64, fromSynchronizer bool) error {
	return bb.localStateDB.Reset(batchNum, fromSynchronizer)
}

// BuildBatch takes the transactions and returns the common.ZKInputs of the next batch
func (bb *BatchBuilder) BuildBatch(configBatch *ConfigBatch, l1usertxs, l1coordinatortxs []*common.L1Tx, l2txs []*common.L2Tx, tokenIDs []common.TokenID) (*common.ZKInputs, error) {
	zkInputs, _, err := bb.localStateDB.ProcessTxs(false, l1usertxs, l1coordinatortxs, l2txs)
	return zkInputs, err
}

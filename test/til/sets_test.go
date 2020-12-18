package til

import (
	"strings"
	"testing"

	"github.com/hermeznetwork/hermez-node/common"
	"github.com/stretchr/testify/assert"
)

func TestCompileSetsBase(t *testing.T) {
	parser := newParser(strings.NewReader(SetBlockchain0))
	_, err := parser.parse()
	assert.NoError(t, err)
	parser = newParser(strings.NewReader(SetPool0))
	_, err = parser.parse()
	assert.NoError(t, err)

	tc := NewContext(common.RollupConstMaxL1UserTx)
	_, err = tc.GenerateBlocks(SetBlockchain0)
	assert.NoError(t, err)
	_, err = tc.GeneratePoolL2Txs(SetPool0)
	assert.NoError(t, err)
}

func TestCompileSetsMinimumFlow(t *testing.T) {
	// minimum flow
	tc := NewContext(common.RollupConstMaxL1UserTx)
	_, err := tc.GenerateBlocks(SetBlockchainMinimumFlow0)
	assert.NoError(t, err)
	_, err = tc.GeneratePoolL2Txs(SetPoolL2MinimumFlow0)
	assert.NoError(t, err)
}

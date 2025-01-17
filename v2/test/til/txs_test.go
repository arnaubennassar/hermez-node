package til

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	"github.com/arnaubennassar/hermez-node/v2/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateKeys(t *testing.T) {
	tc := NewContext(0, common.RollupConstMaxL1UserTx)
	usernames := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}
	tc.generateKeys(usernames)
	debug := false
	if debug {
		for i, username := range usernames {
			fmt.Println(i, username)
			sk := crypto.FromECDSA(tc.Users[username].EthSk)
			fmt.Println("	eth_sk", hex.EncodeToString(sk))
			fmt.Println("	eth_addr", tc.Users[username].Addr)
			fmt.Println("	bjj_sk", hex.EncodeToString(tc.Users[username].BJJ[:]))
			fmt.Println("	bjj_pub", tc.Users[username].BJJ.Public().Compress())
		}
	}
}

func TestGenerateBlocksNoBatches(t *testing.T) {
	set := `
		Type: Blockchain
		AddToken(1)
		AddToken(2)

		CreateAccountDeposit(1) A: 11
		CreateAccountDeposit(2) B: 22

		> block
	`
	tc := NewContext(0, common.RollupConstMaxL1UserTx)
	blocks, err := tc.GenerateBlocks(set)
	require.NoError(t, err)
	assert.Equal(t, 1, len(blocks))
	assert.Equal(t, 0, len(blocks[0].Rollup.Batches))
	assert.Equal(t, 2, len(blocks[0].Rollup.AddedTokens))
	assert.Equal(t, 2, len(blocks[0].Rollup.L1UserTxs))
}

func TestGenerateBlocks(t *testing.T) {
	set := `
		Type: Blockchain
		AddToken(1)
		AddToken(2)
		AddToken(3)
	
		CreateAccountDeposit(1) A: 10
		CreateAccountDeposit(2) A: 20
		CreateAccountDeposit(1) B: 5
		CreateAccountDeposit(1) C: 5
		CreateAccountDeposit(1) D: 5

		> batchL1 // batchNum = 1
		> batchL1 // batchNum = 2

		CreateAccountDepositTransfer(1) F-A: 15, 10

		Transfer(1) A-B: 6 (1)
		Transfer(1) B-D: 3 (1)
		Transfer(1) A-D: 1 (1)

		// set new batch
		> batch // batchNum = 3
		CreateAccountCoordinator(1) E
		CreateAccountCoordinator(2) B

		DepositTransfer(1) A-B: 15, 10
		Transfer(1) C-A : 3 (1)
		Transfer(2) A-B: 15 (1)
		Transfer(1) A-E: 1 (1)

		CreateAccountDeposit(1) User0: 20
		CreateAccountDeposit(3) User1: 20
		CreateAccountCoordinator(1) User1
		CreateAccountCoordinator(3) User0
		> batchL1 // batchNum = 4
		Transfer(1) User0-User1: 15 (1)
		Transfer(3) User1-User0: 15 (1)
		Transfer(1) A-C: 1 (1)

		> batchL1 // batchNum = 5

		Transfer(1) User1-User0: 1 (1)

		> block

		// Exits
		Transfer(1) A-B: 1 (1)
		Exit(1) A: 5 (1)
		
		> batch // batchNum = 6
		> block

		// this transaction should not be generated, as it's after last
		// batch and last block
		Transfer(1) User1-User0: 1 (1)
	`
	tc := NewContext(0, common.RollupConstMaxL1UserTx)
	blocks, err := tc.GenerateBlocks(set)
	require.NoError(t, err)
	assert.Equal(t, 2, len(blocks))
	assert.Equal(t, 5, len(blocks[0].Rollup.Batches))
	assert.Equal(t, 1, len(blocks[1].Rollup.Batches))
	assert.Equal(t, 9, len(blocks[0].Rollup.L1UserTxs))
	assert.Equal(t, 4, len(blocks[0].Rollup.Batches[3].L1CoordinatorTxs))
	assert.Equal(t, 0, len(blocks[1].Rollup.L1UserTxs))

	// Check expected values generated by each line
	// #0: Deposit(1) A: 10
	tc.checkL1TxParams(t, blocks[0].Rollup.L1UserTxs[0], common.TxTypeCreateAccountDeposit, 1,
		"A", "", big.NewInt(10), nil)
	// #1: Deposit(2) A: 20
	tc.checkL1TxParams(t, blocks[0].Rollup.L1UserTxs[1], common.TxTypeCreateAccountDeposit, 2,
		"A", "", big.NewInt(20), nil)
	// // #2: Deposit(1) A: 20
	tc.checkL1TxParams(t, blocks[0].Rollup.L1UserTxs[2], common.TxTypeCreateAccountDeposit, 1,
		"B", "", big.NewInt(5), nil)
	// // #3: CreateAccountDeposit(1) C: 5
	tc.checkL1TxParams(t, blocks[0].Rollup.L1UserTxs[3], common.TxTypeCreateAccountDeposit, 1,
		"C", "", big.NewInt(5), nil)
	// // #4: CreateAccountDeposit(1) D: 5
	tc.checkL1TxParams(t, blocks[0].Rollup.L1UserTxs[4], common.TxTypeCreateAccountDeposit, 1,
		"D", "", big.NewInt(5), nil)
	// #5: Transfer(1) A-B: 6 (1)
	tc.checkL2TxParams(t, blocks[0].Rollup.Batches[2].L2Txs[0], common.TxTypeTransfer, 1, "A",
		"B", big.NewInt(6), common.BatchNum(3))
	// #6: Transfer(1) B-D: 3 (1)
	tc.checkL2TxParams(t, blocks[0].Rollup.Batches[2].L2Txs[1], common.TxTypeTransfer, 1, "B",
		"D", big.NewInt(3), common.BatchNum(3))
	// #7: Transfer(1) A-D: 1 (1)
	tc.checkL2TxParams(t, blocks[0].Rollup.Batches[2].L2Txs[2], common.TxTypeTransfer, 1, "A",
		"D", big.NewInt(1), common.BatchNum(3))
	// change of Batch #8: CreateAccountDepositTransfer(1) F-A: 15, 10 (3)
	tc.checkL1TxParams(t, blocks[0].Rollup.L1UserTxs[5],
		common.TxTypeCreateAccountDepositTransfer, 1, "F", "A", big.NewInt(15), big.NewInt(10))
	// #9: DepositTransfer(1) A-B: 15, 10 (1)
	tc.checkL1TxParams(t, blocks[0].Rollup.L1UserTxs[6], common.TxTypeDepositTransfer, 1, "A",
		"B", big.NewInt(15), big.NewInt(10))
	// #11: Transfer(1) C-A : 3 (1)
	tc.checkL2TxParams(t, blocks[0].Rollup.Batches[3].L2Txs[0], common.TxTypeTransfer, 1, "C",
		"A", big.NewInt(3), common.BatchNum(4))
	// #12: Transfer(2) A-B: 15 (1)
	tc.checkL2TxParams(t, blocks[0].Rollup.Batches[3].L2Txs[1], common.TxTypeTransfer, 2, "A",
		"B", big.NewInt(15), common.BatchNum(4))
	// #13: Deposit(1) User0: 20
	tc.checkL1TxParams(t, blocks[0].Rollup.L1UserTxs[7], common.TxTypeCreateAccountDeposit, 1,
		"User0", "", big.NewInt(20), nil)
	// // #14: Deposit(3) User1: 20
	tc.checkL1TxParams(t, blocks[0].Rollup.L1UserTxs[8], common.TxTypeCreateAccountDeposit, 3,
		"User1", "", big.NewInt(20), nil)
	// #15: Transfer(1) User0-User1: 15 (1)
	tc.checkL2TxParams(t, blocks[0].Rollup.Batches[4].L2Txs[0], common.TxTypeTransfer, 1,
		"User0", "User1", big.NewInt(15), common.BatchNum(5))
	// #16: Transfer(3) User1-User0: 15 (1)
	tc.checkL2TxParams(t, blocks[0].Rollup.Batches[4].L2Txs[1], common.TxTypeTransfer, 3,
		"User1", "User0", big.NewInt(15), common.BatchNum(5))
	// #17: Transfer(1) A-C: 1 (1)
	tc.checkL2TxParams(t, blocks[0].Rollup.Batches[4].L2Txs[2], common.TxTypeTransfer, 1, "A",
		"C", big.NewInt(1), common.BatchNum(5))
	// change of Batch #18: Transfer(1) User1-User0: 1 (1)
	tc.checkL2TxParams(t, blocks[1].Rollup.Batches[0].L2Txs[0], common.TxTypeTransfer, 1,
		"User1", "User0", big.NewInt(1), common.BatchNum(6))
	// change of Block (implies also a change of batch) #19: Transfer(1) A-B: 1 (1)
	tc.checkL2TxParams(t, blocks[1].Rollup.Batches[0].L2Txs[1], common.TxTypeTransfer, 1, "A",
		"B", big.NewInt(1), common.BatchNum(6))
}

func (tc *Context) checkL1TxParams(t *testing.T, tx common.L1Tx, typ common.TxType,
	tokenID common.TokenID, from, to string, depositAmount, amount *big.Int) {
	assert.Equal(t, typ, tx.Type)
	if tx.FromIdx != common.Idx(0) {
		assert.Equal(t, tc.Users[from].Accounts[tokenID].Idx, tx.FromIdx)
	}
	assert.Equal(t, tc.Users[from].Addr.Hex(), tx.FromEthAddr.Hex())
	assert.Equal(t, tc.Users[from].BJJ.Public().Compress(), tx.FromBJJ)
	if tx.ToIdx != common.Idx(0) {
		assert.Equal(t, tc.Users[to].Accounts[tokenID].Idx, tx.ToIdx)
	}
	if depositAmount != nil {
		assert.Equal(t, depositAmount, tx.DepositAmount)
	}
	if amount != nil {
		assert.Equal(t, amount, tx.Amount)
	}
}

func (tc *Context) checkL2TxParams(t *testing.T, tx common.L2Tx, typ common.TxType,
	tokenID common.TokenID, from, to string, amount *big.Int, batchNum common.BatchNum) {
	assert.Equal(t, typ, tx.Type)
	assert.Equal(t, tc.Users[from].Accounts[tokenID].Idx, tx.FromIdx)
	if tx.Type != common.TxTypeExit {
		assert.Equal(t, tc.Users[to].Accounts[tokenID].Idx, tx.ToIdx)
	}
	if amount != nil {
		assert.Equal(t, amount, tx.Amount)
	}
	assert.Equal(t, batchNum, tx.BatchNum)
}

func TestGeneratePoolL2Txs(t *testing.T) {
	set := `
		Type: Blockchain
		AddToken(1)
		AddToken(2)
		AddToken(3)
	
		CreateAccountDeposit(1) A: 10
		CreateAccountDeposit(2) A: 20
		CreateAccountDeposit(1) B: 5
		CreateAccountDeposit(1) C: 5
		CreateAccountDeposit(1) User0: 5
		CreateAccountDeposit(1) User1: 0
		CreateAccountDeposit(3) User0: 0
		CreateAccountDeposit(3) User1: 5
		CreateAccountDeposit(2) B: 5
		CreateAccountDeposit(2) D: 0
		> batchL1
		> batchL1
	`
	tc := NewContext(0, common.RollupConstMaxL1UserTx)
	_, err := tc.GenerateBlocks(set)
	require.NoError(t, err)
	set = `
		Type: PoolL2
		PoolTransfer(1) A-B: 6 (1)
		PoolTransfer(1) B-C: 3 (1)
		PoolTransfer(1) C-A: 3 (1)
		PoolTransfer(1) A-B: 1 (1)
		PoolTransfer(2) A-B: 15 (1)
		PoolTransfer(1) User0-User1: 15 (1)
		PoolTransfer(3) User1-User0: 15 (1)
		PoolTransfer(2) B-D: 3 (1)
		PoolExit(1) A: 3 (1)
		PoolTransferToEthAddr(1) A-B: 1 (1)
		PoolTransferToBJJ(1) A-B: 1 (1)
	`
	poolL2Txs, err := tc.GeneratePoolL2Txs(set)
	require.NoError(t, err)
	assert.Equal(t, 11, len(poolL2Txs))
	assert.Equal(t, common.TxTypeTransfer, poolL2Txs[0].Type)
	assert.Equal(t, common.TxTypeExit, poolL2Txs[8].Type)
	assert.Equal(t, tc.Users["B"].Addr.Hex(), poolL2Txs[9].ToEthAddr.Hex())
	assert.Equal(t, tc.Users["B"].BJJ.Public().String(), poolL2Txs[10].ToBJJ.String())
	assert.Equal(t, common.EmptyAddr.Hex(), poolL2Txs[5].ToEthAddr.Hex())
	assert.Equal(t, common.EmptyBJJComp.String(), poolL2Txs[5].ToBJJ.String())

	assert.Equal(t, common.Nonce(0), poolL2Txs[0].Nonce)
	assert.Equal(t, common.Nonce(0), poolL2Txs[1].Nonce)
	assert.Equal(t, common.Nonce(0), poolL2Txs[2].Nonce)
	assert.Equal(t, common.Nonce(1), poolL2Txs[3].Nonce)
	assert.Equal(t, common.Nonce(0), poolL2Txs[4].Nonce)
	assert.Equal(t, common.Nonce(0), poolL2Txs[5].Nonce)
	assert.Equal(t, common.Nonce(0), poolL2Txs[6].Nonce)
	assert.Equal(t, common.Nonce(0), poolL2Txs[7].Nonce)
	assert.Equal(t, common.Nonce(2), poolL2Txs[8].Nonce)
	assert.Equal(t, common.Nonce(3), poolL2Txs[9].Nonce)

	assert.Equal(t, tc.Users["B"].Addr.Hex(), poolL2Txs[9].ToEthAddr.Hex())
	assert.Equal(t, common.EmptyBJJComp, poolL2Txs[9].ToBJJ)
	assert.Equal(t, common.TxTypeTransferToEthAddr, poolL2Txs[9].Type)
	assert.Equal(t, common.FFAddr, poolL2Txs[10].ToEthAddr)
	assert.Equal(t, tc.Users["B"].BJJ.Public().String(), poolL2Txs[10].ToBJJ.String())
	assert.Equal(t, common.TxTypeTransferToBJJ, poolL2Txs[10].Type)

	// load another set in the same Context
	set = `
		Type: PoolL2
		PoolTransfer(1) A-B: 6 (1)
		PoolTransfer(1) B-C: 3 (1)
		PoolTransfer(1) A-C: 3 (1)
	`
	poolL2Txs, err = tc.GeneratePoolL2Txs(set)
	require.NoError(t, err)
	assert.Equal(t, common.Nonce(5), poolL2Txs[0].Nonce)
	assert.Equal(t, common.Nonce(1), poolL2Txs[1].Nonce)
	assert.Equal(t, common.Nonce(6), poolL2Txs[2].Nonce)

	// check that a PoolL2Tx can be done to a non existing ToIdx
	set = `
		Type: Blockchain
		AddToken(1)
		CreateAccountDeposit(1) A: 10
		> batchL1
		> batchL1
		> block
	`
	tc = NewContext(0, common.RollupConstMaxL1UserTx)
	_, err = tc.GenerateBlocks(set)
	require.NoError(t, err)
	set = `
		Type: PoolL2
		PoolTransferToEthAddr(1) A-B: 3 (1)
		PoolTransferToBJJ(1) A-C: 3 (1)
	`
	_, err = tc.GeneratePoolL2Txs(set)
	require.NoError(t, err)
	// expect error, as FromIdx=B is still not created for TokenID=1
	set = `
		Type: PoolL2
		PoolTransferToEthAddr(1) B-A: 3 (1)
		PoolTransferToBJJ(1) B-A: 3 (1)
	`
	_, err = tc.GeneratePoolL2Txs(set)
	require.NotNil(t, err)
}

func TestGeneratePoolL2TxsFromInstructions(t *testing.T) {
	// Generate necessary L1 data
	set := `
		Type: Blockchain
		AddToken(1)
	
		CreateAccountCoordinator(1) A
		CreateAccountDeposit(1) B: 7
		> batchL1
		> batchL1
	`
	tc := NewContext(0, common.RollupConstMaxL1UserTx)
	_, err := tc.GenerateBlocks(set)
	require.NoError(t, err)

	// Generate Pool txs using instructions
	instructionSet := []Instruction{}
	i := 0
	a := big.NewInt(3)
	instructionSet = append(instructionSet, Instruction{
		LineNum: i,
		// Literal: "PoolTransferToEthAddr(1) B-A: 3 (1)",
		Typ:     common.TxTypeTransferToEthAddr,
		From:    "B",
		To:      "A",
		TokenID: 1,
		Amount:  a,
		Fee:     1,
	})
	i++
	instructionSet = append(instructionSet, Instruction{
		LineNum: i,
		// Literal: "PoolTransferToBJJ(1) B-A: 3 (1)",
		Typ:     common.TxTypeTransferToBJJ,
		From:    "B",
		To:      "A",
		TokenID: 1,
		Amount:  a,
		Fee:     1,
	})
	txsFromInstructions, err := tc.GeneratePoolL2TxsFromInstructions(instructionSet)
	require.NoError(t, err)
	// Generate Pool txs using string
	tc = NewContext(0, common.RollupConstMaxL1UserTx)
	_, err = tc.GenerateBlocks(set)
	require.NoError(t, err)
	stringSet := `
		Type: PoolL2
		PoolTransferToEthAddr(1) B-A: 3 (1)
		PoolTransferToBJJ(1) B-A: 3 (1)
	`
	txsFromString, err := tc.GeneratePoolL2Txs(stringSet)
	require.NoError(t, err)
	// Compare generated txs from instructions and string
	// timestamps will be different
	for i := 0; i < len(txsFromString); i++ {
		txsFromInstructions[i].Timestamp = txsFromString[i].Timestamp
	}
	assert.Equal(t, txsFromString, txsFromInstructions)
}

func TestGenerateErrors(t *testing.T) {
	// unregistered token
	set := `Type: Blockchain
		CreateAccountDeposit(1) A: 5
		> batchL1
		`
	tc := NewContext(0, common.RollupConstMaxL1UserTx)
	_, err := tc.GenerateBlocks(set)
	assert.Equal(t,
		"Line 2: Can not process CreateAccountDeposit: TokenID 1 not registered, "+
			"last registered TokenID: 0", err.Error())

	// ensure AddToken sequentiality and not using 0
	set = `
		Type: Blockchain
		AddToken(0)
	`
	tc = NewContext(0, common.RollupConstMaxL1UserTx)
	_, err = tc.GenerateBlocks(set)
	require.Equal(t, "Line 2: AddToken can not register TokenID 0", err.Error())

	set = `
		Type: Blockchain
		AddToken(2)
	`
	tc = NewContext(0, common.RollupConstMaxL1UserTx)
	_, err = tc.GenerateBlocks(set)
	require.Equal(t, "Line 2: AddToken TokenID should be sequential, expected TokenID: "+
		"1, defined TokenID: 2", err.Error())

	set = `
		Type: Blockchain
		AddToken(1)
		AddToken(2)
		AddToken(3)
		AddToken(5)
	`
	tc = NewContext(0, common.RollupConstMaxL1UserTx)
	_, err = tc.GenerateBlocks(set)
	require.Equal(t, "Line 5: AddToken TokenID should be sequential, expected TokenID: "+
		"4, defined TokenID: 5", err.Error())

	// check transactions when account is not created yet
	set = `
		Type: Blockchain
		AddToken(1)
		CreateAccountDeposit(1) A: 10
		> batchL1
		CreateAccountDeposit(1) B
		Transfer(1) A-B: 6 (1)
		> batch
	`
	tc = NewContext(0, common.RollupConstMaxL1UserTx)
	_, err = tc.GenerateBlocks(set)
	require.Equal(t, "Line 5: CreateAccountDeposit(1)BTransfer(1) A-B: 6 (1)\n, err: "+
		"Expected ':', found 'Transfer'", err.Error())
	set = `
		Type: Blockchain
		AddToken(1)
		CreateAccountDeposit(1) A: 10
		> batchL1
		CreateAccountCoordinator(1) B
		> batchL1
		> batch
		Transfer(1) A-B: 6 (1)
		> batch
	`
	tc = NewContext(0, common.RollupConstMaxL1UserTx)
	_, err = tc.GenerateBlocks(set)
	require.NoError(t, err)

	// check nonces
	set = `
		Type: Blockchain
		AddToken(1)
		CreateAccountDeposit(1) A: 10
		> batchL1
		CreateAccountCoordinator(1) B
		> batchL1
		Transfer(1) A-B: 6 (1)
		Transfer(1) A-B: 6 (1) // on purpose this is moving more money that
				       // what it has in the account, Til should not fail
		Transfer(1) B-A: 6 (1)
		Exit(1) A: 3 (1)
		> batch
	`
	tc = NewContext(0, common.RollupConstMaxL1UserTx)
	_, err = tc.GenerateBlocks(set)
	require.NoError(t, err)
	assert.Equal(t, common.Nonce(3), tc.Users["A"].Accounts[common.TokenID(1)].Nonce)
	assert.Equal(t, common.Idx(256), tc.Users["A"].Accounts[common.TokenID(1)].Idx)
	assert.Equal(t, common.Nonce(1), tc.Users["B"].Accounts[common.TokenID(1)].Nonce)
	assert.Equal(t, common.Idx(257), tc.Users["B"].Accounts[common.TokenID(1)].Idx)
}

func TestGenerateFromInstructions(t *testing.T) {
	// Generate block from instructions
	setInst := []Instruction{}
	i := 0
	setInst = append(setInst, Instruction{
		LineNum: i,
		// Literal: "AddToken(1)",
		Typ:     TypeAddToken,
		TokenID: 1,
	})
	i++
	da := big.NewInt(10)
	setInst = append(setInst, Instruction{
		LineNum: i,
		// Literal: "CreateAccountDeposit(1) A: 10",
		Typ:           common.TxTypeCreateAccountDeposit,
		From:          "A",
		TokenID:       1,
		DepositAmount: da,
	})
	i++
	setInst = append(setInst, Instruction{
		LineNum: i,
		// Literal: "> batchL1",
		Typ: TypeNewBatchL1,
	})
	i++
	setInst = append(setInst, Instruction{
		LineNum: i,
		// Literal: "CreateAccountCoordinator(1) B",
		Typ:     TxTypeCreateAccountDepositCoordinator,
		From:    "B",
		TokenID: 1,
	})
	i++
	setInst = append(setInst, Instruction{
		LineNum: i,
		// Literal: "> batchL1",
		Typ: TypeNewBatchL1,
	})
	i++
	a := big.NewInt(6)
	setInst = append(setInst, Instruction{
		LineNum: i, // 5
		// Literal: "Transfer(1) A-B: 6 (1)",
		Typ:     common.TxTypeTransfer,
		From:    "A",
		To:      "B",
		TokenID: 1,
		Amount:  a,
		Fee:     1,
	})
	i++
	setInst = append(setInst, Instruction{
		LineNum: i,
		// Literal: "Transfer(1) A-B: 6 (1)",
		Typ:     common.TxTypeTransfer,
		From:    "A",
		To:      "B",
		TokenID: 1,
		Amount:  a,
		Fee:     1,
	})
	i++
	setInst = append(setInst, Instruction{
		LineNum: i,
		// Literal: "Transfer(1) B-A: 6 (1)",
		Typ:     common.TxTypeTransfer,
		From:    "B",
		To:      "A",
		TokenID: 1,
		Amount:  a,
		Fee:     1,
	})
	i++
	a = big.NewInt(3)
	setInst = append(setInst, Instruction{
		LineNum: i,
		// Literal: "Exit(1) A: 3 (1)",
		Typ:     common.TxTypeExit,
		From:    "A",
		TokenID: 1,
		Amount:  a,
		Fee:     1,
	})
	i++
	setInst = append(setInst, Instruction{
		LineNum: i,
		// Literal: "> batch",
		Typ: TypeNewBatch,
	})
	setInst = append(setInst, Instruction{
		LineNum: i,
		// Literal: "> block",
		Typ: TypeNewBlock,
	})

	tc := NewContext(0, common.RollupConstMaxL1UserTx)
	blockFromInstructions, err := tc.GenerateBlocksFromInstructions(setInst)
	require.NoError(t, err)

	// Generate block from string
	setString := `
		Type: Blockchain
		AddToken(1)
		CreateAccountDeposit(1) A: 10
		> batchL1
		CreateAccountCoordinator(1) B
		> batchL1
		Transfer(1) A-B: 6 (1)
		Transfer(1) A-B: 6 (1) // on purpose this is moving more money that
				       // what it has in the account, Til should not fail
		Transfer(1) B-A: 6 (1)
		Exit(1) A: 3 (1)
		> batch
		> block
	`
	tc = NewContext(0, common.RollupConstMaxL1UserTx)
	blockFromString, err := tc.GenerateBlocks(setString)
	require.NoError(t, err)

	// Generated data should be equivalent, except for Eth Addrs and BJJs
	for i, strBatch := range blockFromString[0].Rollup.Batches {
		// instBatch := blockFromInstructions[0].Rollup.Batches[i]
		for j := 0; j < len(strBatch.L1CoordinatorTxs); j++ {
			blockFromInstructions[0].Rollup.Batches[i].L1CoordinatorTxs[j].FromEthAddr =
				blockFromString[0].Rollup.Batches[i].L1CoordinatorTxs[j].FromEthAddr
			blockFromInstructions[0].Rollup.Batches[i].L1CoordinatorTxs[j].FromBJJ =
				blockFromString[0].Rollup.Batches[i].L1CoordinatorTxs[j].FromBJJ
		}
		for j := 0; j < len(strBatch.L1UserTxs); j++ {
			blockFromInstructions[0].Rollup.Batches[i].L1UserTxs[j].FromEthAddr =
				blockFromString[0].Rollup.Batches[i].L1UserTxs[j].FromEthAddr
			blockFromInstructions[0].Rollup.Batches[i].L1UserTxs[j].FromBJJ =
				blockFromString[0].Rollup.Batches[i].L1UserTxs[j].FromBJJ
		}
	}
	for i := 0; i < len(blockFromString[0].Rollup.L1UserTxs); i++ {
		blockFromInstructions[0].Rollup.L1UserTxs[i].FromEthAddr =
			blockFromString[0].Rollup.L1UserTxs[i].FromEthAddr
		blockFromInstructions[0].Rollup.L1UserTxs[i].FromBJJ =
			blockFromString[0].Rollup.L1UserTxs[i].FromBJJ
	}
	assert.Equal(t, blockFromString, blockFromInstructions)
}

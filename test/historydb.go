package test

import (
	"errors"
	"fmt"
	"math/big"
	"time"

	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/hermeznetwork/hermez-node/common"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/iden3/go-merkletree"
)

// WARNING: the generators in this file doesn't necessary follow the protocol
// they are intended to check that the parsers between struct <==> DB are correct

// GenBlocks generates block from, to block numbers. WARNING: This is meant for DB/API testing, and may not be fully consistent with the protocol.
func GenBlocks(from, to int64) []common.Block {
	var blocks []common.Block
	for i := from; i < to; i++ {
		blocks = append(blocks, common.Block{
			EthBlockNum: i,
			//nolint:gomnd
			Timestamp: time.Now().Add(time.Second * 13).UTC(),
			Hash:      ethCommon.BigToHash(big.NewInt(int64(i))),
		})
	}
	return blocks
}

// GenTokens generates tokens. WARNING: This is meant for DB/API testing, and may not be fully consistent with the protocol.
func GenTokens(nTokens int, blocks []common.Block) []common.Token {
	tokens := []common.Token{}
	for i := 0; i < nTokens; i++ {
		token := common.Token{
			TokenID:     common.TokenID(i),
			Name:        fmt.Sprint(i),
			Symbol:      fmt.Sprint(i),
			Decimals:    uint64(i),
			EthBlockNum: blocks[i%len(blocks)].EthBlockNum,
			EthAddr:     ethCommon.BigToAddress(big.NewInt(int64(i))),
		}
		tokens = append(tokens, token)
	}
	return tokens
}

// GenBatches generates batches. WARNING: This is meant for DB/API testing, and may not be fully consistent with the protocol.
func GenBatches(nBatches int, blocks []common.Block) []common.Batch {
	batches := []common.Batch{}
	collectedFees := make(map[common.TokenID]*big.Int)
	for i := 0; i < 64; i++ {
		collectedFees[common.TokenID(i)] = big.NewInt(int64(i))
	}
	for i := 0; i < nBatches; i++ {
		batch := common.Batch{
			BatchNum:    common.BatchNum(i + 1),
			EthBlockNum: blocks[i%len(blocks)].EthBlockNum,
			//nolint:gomnd
			ForgerAddr:    ethCommon.BigToAddress(big.NewInt(6886723)),
			CollectedFees: collectedFees,
			StateRoot:     common.Hash([]byte("duhdqlwiucgwqeiu")),
			//nolint:gomnd
			NumAccounts: 30,
			ExitRoot:    common.Hash([]byte("tykertheuhtgenuer3iuw3b")),
			SlotNum:     common.SlotNum(i),
		}
		if i%2 == 0 {
			toForge := new(int64)
			*toForge = int64(i + 1)
			batch.ForgeL1TxsNum = toForge
		}
		batches = append(batches, batch)
	}
	return batches
}

// GenAccounts generates accounts. WARNING: This is meant for DB/API testing, and may not be fully consistent with the protocol.
func GenAccounts(totalAccounts, userAccounts int, tokens []common.Token, userAddr *ethCommon.Address, userBjj *babyjub.PublicKey, batches []common.Batch) []common.Account {
	if totalAccounts < userAccounts {
		panic("totalAccounts must be greater than userAccounts")
	}
	accs := []common.Account{}
	for i := 256; i < 256+totalAccounts; i++ {
		var addr ethCommon.Address
		var pubK *babyjub.PublicKey
		if i < 256+userAccounts {
			addr = *userAddr
			pubK = userBjj
		} else {
			addr = ethCommon.BigToAddress(big.NewInt(int64(i)))
			privK := babyjub.NewRandPrivKey()
			pubK = privK.Public()
		}
		accs = append(accs, common.Account{
			Idx:       common.Idx(i),
			TokenID:   tokens[i%len(tokens)].TokenID,
			EthAddr:   addr,
			BatchNum:  batches[i%len(batches)].BatchNum,
			PublicKey: pubK,
		})
	}
	return accs
}

// GenL1Txs generates L1 txs. WARNING: This is meant for DB/API testing, and may not be fully consistent with the protocol.
func GenL1Txs(
	fromIdx int,
	totalTxs, nUserTxs int,
	userAddr *ethCommon.Address,
	accounts []common.Account,
	tokens []common.Token,
	blocks []common.Block,
	batches []common.Batch,
) ([]common.L1Tx, []common.L1Tx) {
	if totalTxs < nUserTxs {
		panic("totalTxs must be greater than userTxs")
	}
	userTxs := []common.L1Tx{}
	othersTxs := []common.L1Tx{}
	_, nextTxsNum := GetNextToForgeNumAndBatch(batches)
	for i := fromIdx; i < fromIdx+totalTxs; i++ {
		token := tokens[i%len(tokens)]
		amount := big.NewInt(int64(i + 1))
		tx := common.L1Tx{
			Position:    i - fromIdx,
			UserOrigin:  i%2 == 0,
			TokenID:     token.TokenID,
			Amount:      amount,
			LoadAmount:  amount,
			EthBlockNum: blocks[i%len(blocks)].EthBlockNum,
		}
		if tx.UserOrigin {
			n := nextTxsNum
			tx.ToForgeL1TxsNum = &n
		} else {
			tx.BatchNum = &batches[i%len(batches)].BatchNum
		}
		nTx, err := common.NewL1Tx(&tx)
		if err != nil {
			panic(err)
		}
		tx = *nTx
		if !tx.UserOrigin {
			tx.BatchNum = &batches[i%len(batches)].BatchNum
		} else if batches[i%len(batches)].ForgeL1TxsNum != nil {
			// Add already forged txs
			tx.BatchNum = &batches[i%len(batches)].BatchNum
			setFromToAndAppend(fromIdx, tx, i, nUserTxs, userAddr, accounts, &userTxs, &othersTxs)
		} else {
			// Add unforged txs
			n := nextTxsNum
			tx.ToForgeL1TxsNum = &n
			tx.UserOrigin = true
			setFromToAndAppend(fromIdx, tx, i, nUserTxs, userAddr, accounts, &userTxs, &othersTxs)
		}
	}
	return userTxs, othersTxs
}

// GetNextToForgeNumAndBatch returns the next BatchNum and ForgeL1TxsNum to be added
func GetNextToForgeNumAndBatch(batches []common.Batch) (common.BatchNum, int64) {
	batchNum := batches[len(batches)-1].BatchNum + 1
	var toForgeL1TxsNum int64
	found := false
	for i := len(batches) - 1; i >= 0; i-- {
		if batches[i].ForgeL1TxsNum != nil {
			toForgeL1TxsNum = *batches[i].ForgeL1TxsNum + 1
			found = true
			break
		}
	}
	if !found {
		panic("toForgeL1TxsNum not found")
	}
	return batchNum, toForgeL1TxsNum
}

func setFromToAndAppend(
	fromIdx int,
	tx common.L1Tx,
	i, nUserTxs int,
	userAddr *ethCommon.Address,
	accounts []common.Account,
	userTxs *[]common.L1Tx,
	othersTxs *[]common.L1Tx,
) {
	if i < fromIdx+nUserTxs {
		var from, to *common.Account
		var err error
		if i%2 == 0 {
			from, err = randomAccount(i, true, userAddr, accounts)
			if err != nil {
				panic(err)
			}
			to, err = randomAccount(i, false, userAddr, accounts)
			if err != nil {
				panic(err)
			}
		} else {
			from, err = randomAccount(i, false, userAddr, accounts)
			if err != nil {
				panic(err)
			}
			to, err = randomAccount(i, true, userAddr, accounts)
			if err != nil {
				panic(err)
			}
		}
		tx.FromIdx = from.Idx
		tx.FromEthAddr = from.EthAddr
		tx.FromBJJ = from.PublicKey
		tx.ToIdx = to.Idx
		*userTxs = append(*userTxs, tx)
	} else {
		from, err := randomAccount(i, false, userAddr, accounts)
		if err != nil {
			panic(err)
		}
		to, err := randomAccount(i, false, userAddr, accounts)
		if err != nil {
			panic(err)
		}
		tx.FromIdx = from.Idx
		tx.FromEthAddr = from.EthAddr
		tx.FromBJJ = from.PublicKey
		tx.ToIdx = to.Idx
		*othersTxs = append(*othersTxs, tx)
	}
}

// GenL2Txs generates L2 txs. WARNING: This is meant for DB/API testing, and may not be fully consistent with the protocol.
func GenL2Txs(
	fromIdx int,
	totalTxs, nUserTxs int,
	userAddr *ethCommon.Address,
	accounts []common.Account,
	tokens []common.Token,
	blocks []common.Block,
	batches []common.Batch,
) ([]common.L2Tx, []common.L2Tx) {
	if totalTxs < nUserTxs {
		panic("totalTxs must be greater than userTxs")
	}
	userTxs := []common.L2Tx{}
	othersTxs := []common.L2Tx{}
	for i := fromIdx; i < fromIdx+totalTxs; i++ {
		amount := big.NewInt(int64(i + 1))
		fee := common.FeeSelector(i % 256) //nolint:gomnd
		tx := common.L2Tx{
			TxID:        common.TxID([12]byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, byte(i)}), // only for testing purposes
			BatchNum:    batches[i%len(batches)].BatchNum,
			Position:    i - fromIdx,
			Amount:      amount,
			Fee:         fee,
			Nonce:       common.Nonce(i + 1),
			EthBlockNum: blocks[i%len(blocks)].EthBlockNum,
			Type:        randomTxType(i),
		}
		if i < nUserTxs {
			var from, to *common.Account
			var err error
			if i%2 == 0 {
				from, err = randomAccount(i, true, userAddr, accounts)
				if err != nil {
					panic(err)
				}
				to, err = randomAccount(i, false, userAddr, accounts)
				if err != nil {
					panic(err)
				}
			} else {
				from, err = randomAccount(i, false, userAddr, accounts)
				if err != nil {
					panic(err)
				}
				to, err = randomAccount(i, true, userAddr, accounts)
				if err != nil {
					panic(err)
				}
			}
			tx.FromIdx = from.Idx
			tx.ToIdx = to.Idx
		} else {
			from, err := randomAccount(i, false, userAddr, accounts)
			if err != nil {
				panic(err)
			}
			to, err := randomAccount(i, false, userAddr, accounts)
			if err != nil {
				panic(err)
			}
			tx.FromIdx = from.Idx
			tx.ToIdx = to.Idx
		}

		if i < nUserTxs {
			userTxs = append(userTxs, tx)
		} else {
			othersTxs = append(othersTxs, tx)
		}
	}
	return userTxs, othersTxs
}

// GenCoordinators generates coordinators. WARNING: This is meant for DB/API testing, and may not be fully consistent with the protocol.
func GenCoordinators(nCoords int, blocks []common.Block) []common.Coordinator {
	coords := []common.Coordinator{}
	for i := 0; i < nCoords; i++ {
		coords = append(coords, common.Coordinator{
			EthBlockNum: blocks[i%len(blocks)].EthBlockNum,
			Forger:      ethCommon.BigToAddress(big.NewInt(int64(i))),
			Bidder:      ethCommon.BigToAddress(big.NewInt(int64(i))),
			URL:         "https://foo.bar",
		})
	}
	return coords
}

// GenBids generates bids. WARNING: This is meant for DB/API testing, and may not be fully consistent with the protocol.
func GenBids(nBids int, blocks []common.Block, coords []common.Coordinator) []common.Bid {
	bids := []common.Bid{}
	for i := 0; i < nBids; i++ {
		bids = append(bids, common.Bid{
			SlotNum:     common.SlotNum(i),
			BidValue:    big.NewInt(int64(i)),
			EthBlockNum: blocks[i%len(blocks)].EthBlockNum,
			Bidder:      coords[i%len(blocks)].Bidder,
		})
	}
	return bids
}

// GenExitTree generates an exitTree (as an array of Exits)
//nolint:gomnd
func GenExitTree(n int, batches []common.Batch, accounts []common.Account) []common.ExitInfo {
	exitTree := make([]common.ExitInfo, n)
	for i := 0; i < n; i++ {
		exitTree[i] = common.ExitInfo{
			BatchNum:               batches[i%len(batches)].BatchNum,
			InstantWithdrawn:       nil,
			DelayedWithdrawRequest: nil,
			DelayedWithdrawn:       nil,
			AccountIdx:             accounts[i%len(accounts)].Idx,
			MerkleProof: &merkletree.CircomVerifierProof{
				Root: &merkletree.Hash{byte(i), byte(i + 1)},
				Siblings: []*big.Int{
					big.NewInt(int64(i) * 10),
					big.NewInt(int64(i)*100 + 1),
					big.NewInt(int64(i)*1000 + 2)},
				OldKey:   &merkletree.Hash{byte(i * 1), byte(i*1 + 1)},
				OldValue: &merkletree.Hash{byte(i * 2), byte(i*2 + 1)},
				IsOld0:   i%2 == 0,
				Key:      &merkletree.Hash{byte(i * 3), byte(i*3 + 1)},
				Value:    &merkletree.Hash{byte(i * 4), byte(i*4 + 1)},
				Fnc:      i % 2,
			},
			Balance: big.NewInt(int64(i) * 1000),
		}
		if i%2 == 0 {
			instant := new(int64)
			*instant = int64(batches[(i+1)%len(batches)].BatchNum)
			exitTree[i].InstantWithdrawn = instant
		} else if i%3 == 0 {
			delayedReq := new(int64)
			*delayedReq = int64(batches[(i+1)%len(batches)].BatchNum)
			exitTree[i].DelayedWithdrawRequest = delayedReq
			if i%9 == 0 {
				delayed := new(int64)
				*delayed = int64(batches[(i+2)%len(batches)].BatchNum)
				exitTree[i].DelayedWithdrawn = delayed
			}
		}
	}
	return exitTree
}

func randomAccount(seed int, userAccount bool, userAddr *ethCommon.Address, accs []common.Account) (*common.Account, error) {
	i := seed % len(accs)
	firstI := i
	for {
		acc := accs[i]
		if userAccount && *userAddr == acc.EthAddr {
			return &acc, nil
		}
		if !userAccount && (userAddr == nil || *userAddr != acc.EthAddr) {
			return &acc, nil
		}
		i++
		i = i % len(accs)
		if i == firstI {
			return &acc, errors.New("Didnt found any account matchinng the criteria")
		}
	}
}

func randomTxType(seed int) common.TxType {
	//nolint:gomnd
	switch seed % 11 {
	case 0:
		return common.TxTypeExit
	//nolint:gomnd
	case 2:
		return common.TxTypeTransfer
	//nolint:gomnd
	case 3:
		return common.TxTypeDeposit
	//nolint:gomnd
	case 4:
		return common.TxTypeCreateAccountDeposit
	//nolint:gomnd
	case 5:
		return common.TxTypeCreateAccountDepositTransfer
	//nolint:gomnd
	case 6:
		return common.TxTypeDepositTransfer
	//nolint:gomnd
	case 7:
		return common.TxTypeForceTransfer
	//nolint:gomnd
	case 8:
		return common.TxTypeForceExit
	//nolint:gomnd
	case 9:
		return common.TxTypeTransferToEthAddr
	//nolint:gomnd
	case 10:
		return common.TxTypeTransferToBJJ
	default:
		return common.TxTypeTransfer
	}
}
package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/arnaubennassar/hermez-node/v2/common"
	"github.com/arnaubennassar/hermez-node/v2/db"
	"github.com/arnaubennassar/hermez-node/v2/db/historydb"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/mitchellh/copystructure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testPoolTxReceive is a struct to be used to assert the response
// of GET /transactions-pool/:id
type testPoolTxReceive struct {
	ItemID      uint64                 `json:"itemId"`
	TxID        common.TxID            `json:"id"`
	Type        common.TxType          `json:"type"`
	FromIdx     string                 `json:"fromAccountIndex"`
	FromEthAddr *string                `json:"fromHezEthereumAddress"`
	FromBJJ     *string                `json:"fromBJJ"`
	ToIdx       *string                `json:"toAccountIndex"`
	ToEthAddr   *string                `json:"toHezEthereumAddress"`
	ToBJJ       *string                `json:"toBjj"`
	Amount      string                 `json:"amount"`
	Fee         common.FeeSelector     `json:"fee"`
	Nonce       common.Nonce           `json:"nonce"`
	State       common.PoolL2TxState   `json:"state"`
	Signature   babyjub.SignatureComp  `json:"signature"`
	RqFromIdx   *string                `json:"requestFromAccountIndex"`
	RqToIdx     *string                `json:"requestToAccountIndex"`
	RqToEthAddr *string                `json:"requestToHezEthereumAddress"`
	RqToBJJ     *string                `json:"requestToBJJ"`
	RqTokenID   *common.TokenID        `json:"requestTokenId"`
	RqAmount    *string                `json:"requestAmount"`
	RqFee       *common.FeeSelector    `json:"requestFee"`
	RqNonce     *common.Nonce          `json:"requestNonce"`
	BatchNum    *common.BatchNum       `json:"batchNum"`
	Timestamp   time.Time              `json:"timestamp"`
	Token       historydb.TokenWithUSD `json:"token"`
}

type testPoolTxsResponse struct {
	Txs          []testPoolTxReceive `json:"transactions"`
	PendingItems uint64              `json:"pendingItems"`
}

func (t testPoolTxsResponse) GetPending() (pendingItems, lastItemID uint64) {
	if len(t.Txs) == 0 {
		return 0, 0
	}
	pendingItems = t.PendingItems
	lastItemID = t.Txs[len(t.Txs)-1].ItemID
	return pendingItems, lastItemID
}

func (t testPoolTxsResponse) Len() int {
	return len(t.Txs)
}

func (t testPoolTxsResponse) New() Pendinger { return &testPoolTxsResponse{} }

func genTestPoolTxs(
	poolTxs []common.PoolL2Tx,
	tokens []historydb.TokenWithUSD,
	accs []common.Account,
) (poolTxsToSend []common.PoolL2Tx, poolTxsToReceive []testPoolTxReceive) {
	poolTxsToSend = []common.PoolL2Tx{}
	poolTxsToReceive = []testPoolTxReceive{}
	for i := range poolTxs {
		// common.PoolL2Tx ==> poolTxsToSend (add token symbols for proper marshaling)
		token := getTokenByID(poolTxs[i].TokenID, tokens)
		txToSend := poolTxs[i]
		txToSend.TokenSymbol = token.Symbol
		var rqToken historydb.TokenWithUSD
		if poolTxs[i].RqToIdx != 0 {
			rqToken = getTokenByID(poolTxs[i].RqTokenID, tokens)
		}
		poolTxsToSend = append(poolTxsToSend, txToSend)
		// common.PoolL2Tx ==> testPoolTxReceive
		genReceiveTx := testPoolTxReceive{
			TxID:      poolTxs[i].TxID,
			Type:      poolTxs[i].Type,
			FromIdx:   common.IdxToHez(poolTxs[i].FromIdx, token.Symbol),
			Amount:    poolTxs[i].Amount.String(),
			Fee:       poolTxs[i].Fee,
			Nonce:     poolTxs[i].Nonce,
			State:     poolTxs[i].State,
			Signature: poolTxs[i].Signature,
			Timestamp: poolTxs[i].Timestamp,
			Token:     token,
		}
		fromAcc := getAccountByIdx(poolTxs[i].FromIdx, accs)
		fromAddr := common.EthAddrToHez(fromAcc.EthAddr)
		genReceiveTx.FromEthAddr = &fromAddr
		fromBjj := common.BjjToString(fromAcc.BJJ)
		genReceiveTx.FromBJJ = &fromBjj
		toIdx := common.IdxToHez(poolTxs[i].ToIdx, token.Symbol)
		genReceiveTx.ToIdx = &toIdx
		if poolTxs[i].ToEthAddr != common.EmptyAddr {
			toEth := common.EthAddrToHez(poolTxs[i].ToEthAddr)
			genReceiveTx.ToEthAddr = &toEth
		} else if poolTxs[i].ToIdx > 255 {
			acc := getAccountByIdx(poolTxs[i].ToIdx, accs)
			addr := common.EthAddrToHez(acc.EthAddr)
			genReceiveTx.ToEthAddr = &addr
		}
		if poolTxs[i].ToBJJ != common.EmptyBJJComp {
			toBJJ := common.BjjToString(poolTxs[i].ToBJJ)
			genReceiveTx.ToBJJ = &toBJJ
		} else if poolTxs[i].ToIdx > 255 {
			acc := getAccountByIdx(poolTxs[i].ToIdx, accs)
			bjj := common.BjjToString(acc.BJJ)
			genReceiveTx.ToBJJ = &bjj
		}
		if poolTxs[i].RqFromIdx != 0 {
			rqFromIdx := common.IdxToHez(poolTxs[i].RqFromIdx, rqToken.Symbol)
			genReceiveTx.RqFromIdx = &rqFromIdx
			genReceiveTx.RqTokenID = &rqToken.TokenID
			rqAmount := poolTxs[i].RqAmount.String()
			genReceiveTx.RqAmount = &rqAmount
			genReceiveTx.RqFee = &poolTxs[i].RqFee
			genReceiveTx.RqNonce = &poolTxs[i].RqNonce

			if poolTxs[i].RqToIdx != 0 {
				rqToIdx := common.IdxToHez(poolTxs[i].RqToIdx, rqToken.Symbol)
				genReceiveTx.RqToIdx = &rqToIdx
			}
			if poolTxs[i].RqToEthAddr != common.EmptyAddr {
				rqToEth := common.EthAddrToHez(poolTxs[i].RqToEthAddr)
				genReceiveTx.RqToEthAddr = &rqToEth
			}
			if poolTxs[i].RqToBJJ != common.EmptyBJJComp {
				rqToBJJ := common.BjjToString(poolTxs[i].RqToBJJ)
				genReceiveTx.RqToBJJ = &rqToBJJ
			}
		}

		poolTxsToReceive = append(poolTxsToReceive, genReceiveTx)
	}
	return poolTxsToSend, poolTxsToReceive
}

func TestPoolTxs(t *testing.T) {
	// POST
	endpoint := apiURL + "transactions-pool"
	fetchedTxID := common.TxID{}
	for _, tx := range tc.poolTxsToSend {
		jsonTxBytes, err := json.Marshal(tx)
		require.NoError(t, err)
		jsonTxReader := bytes.NewReader(jsonTxBytes)
		require.NoError(
			t, doGoodReq(
				"POST",
				endpoint,
				jsonTxReader, &fetchedTxID,
			),
		)
		assert.Equal(t, tx.TxID, fetchedTxID)
	}
	// 400
	// Wrong fee
	badTx := tc.poolTxsToSend[0]
	badTx.Amount = big.NewInt(99950000000000000)
	badTx.Fee = 255
	jsonTxBytes, err := json.Marshal(badTx)
	require.NoError(t, err)
	jsonTxReader := bytes.NewReader(jsonTxBytes)
	err = doBadReq("POST", endpoint, jsonTxReader, 400)
	require.NoError(t, err)
	// Wrong signature
	badTx = tc.poolTxsToSend[0]
	badTx.FromIdx = 1000
	jsonTxBytes, err = json.Marshal(badTx)
	require.NoError(t, err)
	jsonTxReader = bytes.NewReader(jsonTxBytes)
	err = doBadReq("POST", endpoint, jsonTxReader, 400)
	require.NoError(t, err)
	// Wrong to
	badTx = tc.poolTxsToSend[0]
	badTx.ToEthAddr = common.FFAddr
	badTx.ToIdx = 0
	jsonTxBytes, err = json.Marshal(badTx)
	require.NoError(t, err)
	jsonTxReader = bytes.NewReader(jsonTxBytes)
	err = doBadReq("POST", endpoint, jsonTxReader, 400)
	require.NoError(t, err)
	// Wrong rq
	badTx = tc.poolTxsToSend[0]
	badTx.RqFromIdx = 30
	jsonTxBytes, err = json.Marshal(badTx)
	require.NoError(t, err)
	jsonTxReader = bytes.NewReader(jsonTxBytes)
	err = doBadReq("POST", endpoint, jsonTxReader, 409)
	require.NoError(t, err)
	// Wrong maxNumBatch
	badTx = tc.poolTxsToSend[0]
	badTx.MaxNumBatch = 30
	jsonTxBytes, err = json.Marshal(badTx)
	require.NoError(t, err)
	jsonTxReader = bytes.NewReader(jsonTxBytes)
	err = doBadReq("POST", endpoint, jsonTxReader, 400)
	require.NoError(t, err)
	// GET
	// init structures
	fetchedTxsTotal := []testPoolTxReceive{}
	appendIterTotal := func(intr interface{}) {
		for i := 0; i < len(intr.(*testPoolTxsResponse).Txs); i++ {
			tmp, err := copystructure.Copy(intr.(*testPoolTxsResponse).Txs[i])
			if err != nil {
				panic(err)
			}
			fetchedTxsTotal = append(fetchedTxsTotal, tmp.(testPoolTxReceive))
		}
	}
	// get all (no filters)
	limit := 20
	totalAmountOfTransactions := 4
	path := fmt.Sprintf("%s?limit=%d", endpoint, limit)
	require.NoError(t, doGoodReqPaginated(path, db.OrderAsc, &testPoolTxsResponse{}, appendIterTotal))
	assert.Equal(t, totalAmountOfTransactions, len(fetchedTxsTotal))

	account := tc.accounts[2]
	fetchedTxs := []testPoolTxReceive{}
	appendIter := func(intr interface{}) {
		for i := 0; i < len(intr.(*testPoolTxsResponse).Txs); i++ {
			tmp, err := copystructure.Copy(intr.(*testPoolTxsResponse).Txs[i])
			if err != nil {
				panic(err)
			}
			fetchedTxs = append(fetchedTxs, tmp.(testPoolTxReceive))
		}
	}
	// get to check correct behavior with pending items
	// if limit not working correctly, then this is failing with panic
	fetchedTxsTotal = []testPoolTxReceive{}
	limit = 1
	path = fmt.Sprintf("%s?limit=%d", endpoint, limit)
	require.NoError(t, doGoodReqPaginated(path, db.OrderAsc, &testPoolTxsResponse{}, appendIterTotal))
	// get by ethAddr
	limit = 5
	path = fmt.Sprintf("%s?hezEthereumAddress=%s&limit=%d", endpoint, account.EthAddr, limit)
	require.NoError(t, doGoodReqPaginated(path, db.OrderAsc, &testPoolTxsResponse{}, appendIter))
	for _, v := range fetchedTxs {
		isPresent := false
		if string(account.EthAddr) == *v.FromEthAddr || string(account.EthAddr) == *v.ToEthAddr {
			isPresent = true
		}
		assert.True(t, isPresent)
	}
	count := 0
	for _, v := range fetchedTxsTotal {
		if string(account.EthAddr) == *v.FromEthAddr || (v.ToEthAddr != nil && string(account.EthAddr) == *v.ToEthAddr) {
			count++
		}
	}
	assert.Equal(t, count, len(fetchedTxs))
	// get by fromEthAddr
	fetchedTxs = []testPoolTxReceive{}
	path = fmt.Sprintf("%s?fromHezEthereumAddress=%s&limit=%d", endpoint, account.EthAddr, limit)
	require.NoError(t, doGoodReqPaginated(path, db.OrderAsc, &testPoolTxsResponse{}, appendIter))
	for _, v := range fetchedTxs {
		assert.Equal(t, string(account.EthAddr), *v.FromEthAddr)
	}
	count = 0
	for _, v := range fetchedTxsTotal {
		if string(account.EthAddr) == *v.FromEthAddr {
			count++
		}
	}
	assert.Equal(t, count, len(fetchedTxs))
	// get by toEthAddr
	fetchedTxs = []testPoolTxReceive{}
	path = fmt.Sprintf("%s?toHezEthereumAddress=%s&limit=%d", endpoint, account.EthAddr, limit)
	require.NoError(t, doGoodReqPaginated(path, db.OrderAsc, &testPoolTxsResponse{}, appendIter))
	for _, v := range fetchedTxs {
		assert.Equal(t, string(account.EthAddr), *v.ToEthAddr)
	}
	count = 0
	for _, v := range fetchedTxsTotal {
		if v.ToEthAddr != nil && string(account.EthAddr) == *v.ToEthAddr {
			count++
		}
	}
	assert.Equal(t, count, len(fetchedTxs))
	fetchedTxs = []testPoolTxReceive{}
	path = fmt.Sprintf("%s?tokenId=%d&limit=%d", endpoint, account.Token.TokenID, limit)
	require.NoError(t, doGoodReqPaginated(path, db.OrderAsc, &testPoolTxsResponse{}, appendIter))
	for _, v := range fetchedTxs {
		assert.Equal(t, account.Token.TokenID, v.Token.TokenID)
	}
	count = 0
	for _, v := range fetchedTxsTotal {
		if account.Token.TokenID == v.Token.TokenID {
			count++
		}
	}
	assert.Equal(t, count, len(fetchedTxs))
	// get by bjj
	fetchedTxs = []testPoolTxReceive{}
	path = fmt.Sprintf("%s?BJJ=%s&limit=%d", endpoint, account.PublicKey, limit)
	require.NoError(t, doGoodReqPaginated(path, db.OrderAsc, &testPoolTxsResponse{}, appendIter))
	for _, v := range fetchedTxs {
		isPresent := false
		if string(account.PublicKey) == *v.FromBJJ || string(account.PublicKey) == *v.ToBJJ {
			isPresent = true
		}
		assert.True(t, isPresent)
	}
	count = 0
	for _, v := range fetchedTxsTotal {
		if string(account.PublicKey) == *v.FromBJJ || (v.ToBJJ != nil && string(account.PublicKey) == *v.ToBJJ) {
			count++
		}
	}
	assert.Equal(t, count, len(fetchedTxs))

	// get by fromBjj
	fetchedTxs = []testPoolTxReceive{}
	path = fmt.Sprintf("%s?fromBJJ=%s&limit=%d", endpoint, account.PublicKey, limit)
	require.NoError(t, doGoodReqPaginated(path, db.OrderAsc, &testPoolTxsResponse{}, appendIter))
	for _, v := range fetchedTxs {
		assert.Equal(t, string(account.PublicKey), *v.FromBJJ)
	}
	count = 0
	for _, v := range fetchedTxsTotal {
		if string(account.PublicKey) == *v.FromBJJ {
			count++
		}
	}
	assert.Equal(t, count, len(fetchedTxs))
	// get by toBjj
	fetchedTxs = []testPoolTxReceive{}
	path = fmt.Sprintf("%s?toBJJ=%s&limit=%d", endpoint, account.PublicKey, limit)
	require.NoError(t, doGoodReqPaginated(path, db.OrderAsc, &testPoolTxsResponse{}, appendIter))
	for _, v := range fetchedTxs {
		assert.Equal(t, string(account.PublicKey), *v.ToBJJ)
	}
	count = 0
	for _, v := range fetchedTxsTotal {
		if v.ToBJJ != nil && string(account.PublicKey) == *v.ToBJJ {
			count++
		}
	}
	assert.Equal(t, count, len(fetchedTxs))
	// get by fromAccountIndex
	fetchedTxs = []testPoolTxReceive{}
	require.NoError(t, doGoodReqPaginated(
		endpoint+"?fromAccountIndex=hez:ETH:263&limit=10", db.OrderAsc, &testPoolTxsResponse{}, appendIter))
	assert.Equal(t, 1, len(fetchedTxs))
	assert.Equal(t, "hez:ETH:263", fetchedTxs[0].FromIdx)
	// get by toAccountIndex
	fetchedTxs = []testPoolTxReceive{}
	require.NoError(t, doGoodReqPaginated(
		endpoint+"?toAccountIndex=hez:ETH:262&limit=10", db.OrderAsc, &testPoolTxsResponse{}, appendIter))
	assert.Equal(t, 1, len(fetchedTxs))
	toIdx := "hez:ETH:262"
	assert.Equal(t, &toIdx, fetchedTxs[0].ToIdx)
	// get by accountIndex
	fetchedTxs = []testPoolTxReceive{}
	idx := "hez:ETH:259"
	path = fmt.Sprintf("%s?accountIndex=%s&limit=%d", endpoint, idx, limit)
	require.NoError(t, doGoodReqPaginated(
		path, db.OrderAsc, &testPoolTxsResponse{}, appendIter))
	assert.NoError(t, err)
	for _, v := range fetchedTxs {
		isPresent := false
		if v.FromIdx == idx || v.ToIdx == &idx {
			isPresent = true
		}
		assert.True(t, isPresent)
	}
	txTypes := []common.TxType{
		common.TxTypeExit,
		common.TxTypeTransfer,
		common.TxTypeDeposit,
		common.TxTypeCreateAccountDeposit,
		common.TxTypeCreateAccountDepositTransfer,
		common.TxTypeDepositTransfer,
		common.TxTypeForceTransfer,
		common.TxTypeForceExit,
	}
	for _, txType := range txTypes {
		fetchedTxs = []testPoolTxReceive{}
		limit = 2
		path = fmt.Sprintf("%s?type=%s&limit=%d",
			endpoint, txType, limit)
		assert.NoError(t, doGoodReqPaginated(path, db.OrderAsc, &testPoolTxsResponse{}, appendIter))
		for _, v := range fetchedTxs {
			assert.Equal(t, txType, v.Type)
		}
	}

	// get by state
	fetchedTxs = []testPoolTxReceive{}
	require.NoError(t, doGoodReqPaginated(
		endpoint+"?state=pend&limit=10", db.OrderAsc, &testPoolTxsResponse{}, appendIter))
	assert.Equal(t, 4, len(fetchedTxs))
	for _, v := range fetchedTxs {
		assert.Equal(t, common.PoolL2TxStatePending, v.State)
	}
	// GET
	endpoint += "/"
	for _, tx := range tc.poolTxsToReceive {
		fetchedTx := testPoolTxReceive{}
		require.NoError(
			t, doGoodReq(
				"GET",
				endpoint+tx.TxID.String(),
				nil, &fetchedTx,
			),
		)
		assertPoolTx(t, tx, fetchedTx)
	}
	// 400, due invalid TxID
	err = doBadReq("GET", endpoint+"0xG2241b6f2b1dd772dba391f4a1a3407c7c21f598d86e2585a14e616fb4a255f823", nil, 400)
	require.NoError(t, err)
	// 404, due nonexistent TxID in DB
	err = doBadReq("GET", endpoint+"0x02241b6f2b1dd772dba391f4a1a3407c7c21f598d86e2585a14e616fb4a255f823", nil, 404)
	require.NoError(t, err)
}

func assertPoolTx(t *testing.T, expected, actual testPoolTxReceive) {
	// state should be pending
	assert.Equal(t, common.PoolL2TxStatePending, actual.State)
	expected.State = actual.State
	actual.Token.ItemID = 0
	actual.ItemID = 0
	// timestamp should be very close to now
	assert.Less(t, time.Now().UTC().Unix()-3, actual.Timestamp.Unix())
	expected.Timestamp = actual.Timestamp
	// token timestamp
	if expected.Token.USDUpdate == nil {
		assert.Equal(t, expected.Token.USDUpdate, actual.Token.USDUpdate)
	} else {
		assert.Equal(t, expected.Token.USDUpdate.Unix(), actual.Token.USDUpdate.Unix())
		expected.Token.USDUpdate = actual.Token.USDUpdate
	}
	assert.Equal(t, expected, actual)
}

// TestAllTosNull test that the API doesn't accept txs with all the TOs set to null (to eth, to bjj, to idx)
func TestAllTosNull(t *testing.T) {
	// Generate keys
	addr, sk := generateKeys(4444)
	// Generate account:
	var testIdx common.Idx = 333
	account := common.Account{
		Idx:      testIdx,
		TokenID:  0,
		BatchNum: 1,
		BJJ:      sk.Public().Compress(),
		EthAddr:  addr,
		Nonce:    0,
		Balance:  big.NewInt(1000000),
	}
	// Add account to history DB (required to verify signature)
	err := api.h.AddAccounts([]common.Account{account})
	assert.NoError(t, err)
	// Genrate tx with all tos set to nil (to eth, to bjj, to idx)
	tx := common.PoolL2Tx{
		FromIdx: account.Idx,
		TokenID: account.TokenID,
		Amount:  big.NewInt(1000),
		Fee:     200,
		Nonce:   0,
	}
	// Set idx and type manually, and check that the function doesn't allow it
	_, err = common.NewPoolL2Tx(&tx)
	assert.Error(t, err)
	tx.Type = common.TxTypeTransfer
	var txID common.TxID
	txIDRaw, err := hex.DecodeString("02e66e24f7f25272906647c8fd1d7fe8acf3cf3e9b38ffc9f94bbb5090dc275073")
	assert.NoError(t, err)
	copy(txID[:], txIDRaw)
	tx.TxID = txID
	// Sign tx
	toSign, err := tx.HashToSign(0)
	assert.NoError(t, err)
	sig := sk.SignPoseidon(toSign)
	tx.Signature = sig.Compress()
	// Add token symbol for mashaling
	tx.TokenSymbol = "ETH"
	// Send tx to the API
	jsonTxBytes, err := json.Marshal(tx)
	require.NoError(t, err)
	jsonTxReader := bytes.NewReader(jsonTxBytes)
	err = doBadReq("POST", apiURL+"transactions-pool", jsonTxReader, 400)
	require.NoError(t, err)
	// Clean historyDB: the added account shouldn't be there for other tests
	_, err = api.h.DB().DB.Exec(
		fmt.Sprintf("delete from account where idx = %d", testIdx),
	)
	assert.NoError(t, err)
}

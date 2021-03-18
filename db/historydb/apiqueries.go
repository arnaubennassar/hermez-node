package historydb

import (
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"time"

	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/hermeznetwork/hermez-node/apitypes"
	"github.com/hermeznetwork/hermez-node/common"
	"github.com/hermeznetwork/hermez-node/db"
	"github.com/hermeznetwork/tracerr"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/jmoiron/sqlx"
	"github.com/russross/meddler"
)

// GetLastBlockAPI retrieve the block with the highest block number from the DB
func (hdb *HistoryDB) GetLastBlockAPI() (*common.Block, error) {
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	return hdb.GetLastBlock()
}

// GetBatchAPI return the batch with the given batchNum
func (hdb *HistoryDB) GetBatchAPI(batchNum common.BatchNum) (*BatchAPI, error) {
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	return hdb.getBatchAPI(hdb.dbRead, batchNum)
}

// GetBatchInternalAPI return the batch with the given batchNum
func (hdb *HistoryDB) GetBatchInternalAPI(batchNum common.BatchNum) (*BatchAPI, error) {
	return hdb.getBatchAPI(hdb.dbRead, batchNum)
}

func (hdb *HistoryDB) getBatchAPI(d meddler.DB, batchNum common.BatchNum) (*BatchAPI, error) {
	batch := &BatchAPI{}
	if err := meddler.QueryRow(
		d, batch,
		`SELECT batch.item_id, batch.batch_num, batch.eth_block_num,
		batch.forger_addr, batch.fees_collected, batch.total_fees_usd, batch.state_root,
		batch.num_accounts, batch.exit_root, batch.forge_l1_txs_num, batch.slot_num,
		block.timestamp, block.hash,
	    COALESCE ((SELECT COUNT(*) FROM tx WHERE batch_num = batch.batch_num), 0) AS forged_txs
	    FROM batch INNER JOIN block ON batch.eth_block_num = block.eth_block_num
	 	WHERE batch_num = $1;`, batchNum,
	); err != nil {
		return nil, tracerr.Wrap(err)
	}
	batch.CollectedFeesAPI = apitypes.NewCollectedFeesAPI(batch.CollectedFeesDB)
	return batch, nil
}

// GetBatchesAPI return the batches applying the given filters
func (hdb *HistoryDB) GetBatchesAPI(
	minBatchNum, maxBatchNum, slotNum *uint,
	forgerAddr *ethCommon.Address,
	fromItem, limit *uint, order string,
) ([]BatchAPI, uint64, error) {
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	var query string
	var args []interface{}
	queryStr := `SELECT batch.item_id, batch.batch_num, batch.eth_block_num,
	batch.forger_addr, batch.fees_collected, batch.total_fees_usd, batch.state_root,
	batch.num_accounts, batch.exit_root, batch.forge_l1_txs_num, batch.slot_num,
	block.timestamp, block.hash,
	COALESCE ((SELECT COUNT(*) FROM tx WHERE batch_num = batch.batch_num), 0) AS forged_txs,
	count(*) OVER() AS total_items
	FROM batch INNER JOIN block ON batch.eth_block_num = block.eth_block_num `
	// Apply filters
	nextIsAnd := false
	// minBatchNum filter
	if minBatchNum != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "batch.batch_num > ? "
		args = append(args, minBatchNum)
		nextIsAnd = true
	}
	// maxBatchNum filter
	if maxBatchNum != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "batch.batch_num < ? "
		args = append(args, maxBatchNum)
		nextIsAnd = true
	}
	// slotNum filter
	if slotNum != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "batch.slot_num = ? "
		args = append(args, slotNum)
		nextIsAnd = true
	}
	// forgerAddr filter
	if forgerAddr != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "batch.forger_addr = ? "
		args = append(args, forgerAddr)
		nextIsAnd = true
	}
	// pagination
	if fromItem != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		if order == OrderAsc {
			queryStr += "batch.item_id >= ? "
		} else {
			queryStr += "batch.item_id <= ? "
		}
		args = append(args, fromItem)
	}
	queryStr += "ORDER BY batch.item_id "
	if order == OrderAsc {
		queryStr += " ASC "
	} else {
		queryStr += " DESC "
	}
	queryStr += fmt.Sprintf("LIMIT %d;", *limit)
	query = hdb.dbRead.Rebind(queryStr)
	// log.Debug(query)
	batchPtrs := []*BatchAPI{}
	if err := meddler.QueryAll(hdb.dbRead, &batchPtrs, query, args...); err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	batches := db.SlicePtrsToSlice(batchPtrs).([]BatchAPI)
	if len(batches) == 0 {
		return batches, 0, nil
	}
	for i := range batches {
		batches[i].CollectedFeesAPI = apitypes.NewCollectedFeesAPI(batches[i].CollectedFeesDB)
	}
	return batches, batches[0].TotalItems - uint64(len(batches)), nil
}

// GetBestBidAPI returns the best bid in specific slot by slotNum
func (hdb *HistoryDB) GetBestBidAPI(slotNum *int64) (BidAPI, error) {
	bid := &BidAPI{}
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return *bid, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	err = meddler.QueryRow(
		hdb.dbRead, bid, `SELECT bid.*, block.timestamp, coordinator.forger_addr, coordinator.url 
		FROM bid INNER JOIN block ON bid.eth_block_num = block.eth_block_num
		INNER JOIN (
			SELECT bidder_addr, MAX(item_id) AS item_id FROM coordinator
			GROUP BY bidder_addr
		) c ON bid.bidder_addr = c.bidder_addr 
		INNER JOIN coordinator ON c.item_id = coordinator.item_id 
		WHERE slot_num = $1 ORDER BY item_id DESC LIMIT 1;`, slotNum,
	)
	return *bid, tracerr.Wrap(err)
}

// GetBestBidsAPI returns the best bid in specific slot by slotNum
func (hdb *HistoryDB) GetBestBidsAPI(
	minSlotNum, maxSlotNum *int64,
	bidderAddr *ethCommon.Address,
	limit *uint, order string,
) ([]BidAPI, uint64, error) {
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	return hdb.getBestBidsAPI(hdb.dbRead, minSlotNum, maxSlotNum, bidderAddr, limit, order)
}
func (hdb *HistoryDB) getBestBidsAPI(
	d meddler.DB,
	minSlotNum, maxSlotNum *int64,
	bidderAddr *ethCommon.Address,
	limit *uint, order string,
) ([]BidAPI, uint64, error) {
	var query string
	var args []interface{}
	// JOIN the best bid of each slot with the latest update of each coordinator
	queryStr := `SELECT b.*, block.timestamp, coordinator.forger_addr, coordinator.url, 
	COUNT(*) OVER() AS total_items FROM (
	   SELECT slot_num, MAX(item_id) as maxitem 
	   FROM bid GROUP BY slot_num
	)
	AS x INNER JOIN bid AS b ON b.item_id = x.maxitem
	INNER JOIN block ON b.eth_block_num = block.eth_block_num
	INNER JOIN (
		SELECT bidder_addr, MAX(item_id) AS item_id FROM coordinator
		GROUP BY bidder_addr
	) c ON b.bidder_addr = c.bidder_addr 
	INNER JOIN coordinator ON c.item_id = coordinator.item_id 
	WHERE (b.slot_num >= ? AND b.slot_num <= ?)`
	args = append(args, minSlotNum)
	args = append(args, maxSlotNum)
	// Apply filters
	if bidderAddr != nil {
		queryStr += " AND b.bidder_addr = ? "
		args = append(args, bidderAddr)
	}
	queryStr += " ORDER BY b.slot_num "
	if order == OrderAsc {
		queryStr += "ASC "
	} else {
		queryStr += "DESC "
	}
	if limit != nil {
		queryStr += fmt.Sprintf("LIMIT %d;", *limit)
	}
	query = hdb.dbRead.Rebind(queryStr)
	bidPtrs := []*BidAPI{}
	if err := meddler.QueryAll(d, &bidPtrs, query, args...); err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	// log.Debug(query)
	bids := db.SlicePtrsToSlice(bidPtrs).([]BidAPI)
	if len(bids) == 0 {
		return bids, 0, nil
	}
	return bids, bids[0].TotalItems - uint64(len(bids)), nil
}

// GetBidsAPI return the bids applying the given filters
func (hdb *HistoryDB) GetBidsAPI(
	slotNum *int64, bidderAddr *ethCommon.Address,
	fromItem, limit *uint, order string,
) ([]BidAPI, uint64, error) {
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	var query string
	var args []interface{}
	// JOIN each bid with the latest update of each coordinator
	queryStr := `SELECT bid.*, block.timestamp, coord.forger_addr, coord.url, 
	COUNT(*) OVER() AS total_items
	FROM bid INNER JOIN block ON bid.eth_block_num = block.eth_block_num 
	INNER JOIN (
		SELECT bidder_addr, MAX(item_id) AS item_id FROM coordinator
		GROUP BY bidder_addr
	) c ON bid.bidder_addr = c.bidder_addr 
	INNER JOIN coordinator coord ON c.item_id = coord.item_id `
	// Apply filters
	nextIsAnd := false
	// slotNum filter
	if slotNum != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "bid.slot_num = ? "
		args = append(args, slotNum)
		nextIsAnd = true
	}
	// bidder filter
	if bidderAddr != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "bid.bidder_addr = ? "
		args = append(args, bidderAddr)
		nextIsAnd = true
	}
	if fromItem != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		if order == OrderAsc {
			queryStr += "bid.item_id >= ? "
		} else {
			queryStr += "bid.item_id <= ? "
		}
		args = append(args, fromItem)
	}
	// pagination
	queryStr += "ORDER BY bid.item_id "
	if order == OrderAsc {
		queryStr += "ASC "
	} else {
		queryStr += "DESC "
	}
	queryStr += fmt.Sprintf("LIMIT %d;", *limit)
	query, argsQ, err := sqlx.In(queryStr, args...)
	if err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	query = hdb.dbRead.Rebind(query)
	bids := []*BidAPI{}
	if err := meddler.QueryAll(hdb.dbRead, &bids, query, argsQ...); err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	if len(bids) == 0 {
		return []BidAPI{}, 0, nil
	}
	return db.SlicePtrsToSlice(bids).([]BidAPI), bids[0].TotalItems - uint64(len(bids)), nil
}

// GetTokenAPI returns a token from the DB given a TokenID
func (hdb *HistoryDB) GetTokenAPI(tokenID common.TokenID) (*TokenWithUSD, error) {
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	return hdb.GetToken(tokenID)
}

// GetTokensAPI returns a list of tokens from the DB
func (hdb *HistoryDB) GetTokensAPI(
	ids []common.TokenID, symbols []string, name string, fromItem,
	limit *uint, order string,
) ([]TokenWithUSD, uint64, error) {
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	var query string
	var args []interface{}
	queryStr := `SELECT * , COUNT(*) OVER() AS total_items FROM token `
	// Apply filters
	nextIsAnd := false
	if len(ids) > 0 {
		queryStr += "WHERE token_id IN (?) "
		nextIsAnd = true
		args = append(args, ids)
	}
	if len(symbols) > 0 {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "symbol IN (?) "
		args = append(args, symbols)
		nextIsAnd = true
	}
	if name != "" {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "name ~ ? "
		args = append(args, name)
		nextIsAnd = true
	}
	if fromItem != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		if order == OrderAsc {
			queryStr += "item_id >= ? "
		} else {
			queryStr += "item_id <= ? "
		}
		args = append(args, fromItem)
	}
	// pagination
	queryStr += "ORDER BY item_id "
	if order == OrderAsc {
		queryStr += "ASC "
	} else {
		queryStr += "DESC "
	}
	queryStr += fmt.Sprintf("LIMIT %d;", *limit)
	query, argsQ, err := sqlx.In(queryStr, args...)
	if err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	query = hdb.dbRead.Rebind(query)
	tokens := []*TokenWithUSD{}
	if err := meddler.QueryAll(hdb.dbRead, &tokens, query, argsQ...); err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	if len(tokens) == 0 {
		return []TokenWithUSD{}, 0, nil
	}
	return db.SlicePtrsToSlice(tokens).([]TokenWithUSD), uint64(len(tokens)) - tokens[0].TotalItems, nil
}

// GetTxAPI returns a tx from the DB given a TxID
func (hdb *HistoryDB) GetTxAPI(txID common.TxID) (*TxAPI, error) {
	// Warning: amount_success and deposit_amount_success have true as default for
	// performance reasons. The expected default value is false (when txs are unforged)
	// this case is handled at the function func (tx TxAPI) MarshalJSON() ([]byte, error)
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	tx := &TxAPI{}
	err = meddler.QueryRow(
		hdb.dbRead, tx, `SELECT tx.item_id, tx.is_l1, tx.id, tx.type, tx.position, 
		hez_idx(tx.effective_from_idx, token.symbol) AS from_idx, tx.from_eth_addr, tx.from_bjj,
		hez_idx(tx.to_idx, token.symbol) AS to_idx, tx.to_eth_addr, tx.to_bjj,
		tx.amount, tx.amount_success, tx.token_id, tx.amount_usd, 
		tx.batch_num, tx.eth_block_num, tx.to_forge_l1_txs_num, tx.user_origin, 
		tx.deposit_amount, tx.deposit_amount_usd, tx.deposit_amount_success, tx.fee, tx.fee_usd, tx.nonce,
		token.token_id, token.item_id AS token_item_id, token.eth_block_num AS token_block,
		token.eth_addr, token.name, token.symbol, token.decimals, token.usd,
		token.usd_update, block.timestamp
		FROM tx INNER JOIN token ON tx.token_id = token.token_id 
		INNER JOIN block ON tx.eth_block_num = block.eth_block_num 
		WHERE tx.id = $1;`, txID,
	)
	return tx, tracerr.Wrap(err)
}

// GetTxsAPI returns a list of txs from the DB using the HistoryTx struct
// and pagination info
func (hdb *HistoryDB) GetTxsAPI(
	ethAddr *ethCommon.Address, bjj *babyjub.PublicKeyComp,
	tokenID *common.TokenID, idx *common.Idx, batchNum *uint, txType *common.TxType,
	fromItem, limit *uint, order string,
) ([]TxAPI, uint64, error) {
	// Warning: amount_success and deposit_amount_success have true as default for
	// performance reasons. The expected default value is false (when txs are unforged)
	// this case is handled at the function func (tx TxAPI) MarshalJSON() ([]byte, error)
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	if ethAddr != nil && bjj != nil {
		return nil, 0, tracerr.Wrap(errors.New("ethAddr and bjj are incompatible"))
	}
	var query string
	var args []interface{}
	queryStr := `SELECT tx.item_id, tx.is_l1, tx.id, tx.type, tx.position, 
	hez_idx(tx.effective_from_idx, token.symbol) AS from_idx, tx.from_eth_addr, tx.from_bjj,
	hez_idx(tx.to_idx, token.symbol) AS to_idx, tx.to_eth_addr, tx.to_bjj,
	tx.amount, tx.amount_success, tx.token_id, tx.amount_usd, 
	tx.batch_num, tx.eth_block_num, tx.to_forge_l1_txs_num, tx.user_origin, 
	tx.deposit_amount, tx.deposit_amount_usd, tx.deposit_amount_success, tx.fee, tx.fee_usd, tx.nonce,
	token.token_id, token.item_id AS token_item_id, token.eth_block_num AS token_block,
	token.eth_addr, token.name, token.symbol, token.decimals, token.usd,
	token.usd_update, block.timestamp, count(*) OVER() AS total_items 
	FROM tx INNER JOIN token ON tx.token_id = token.token_id 
	INNER JOIN block ON tx.eth_block_num = block.eth_block_num `
	// Apply filters
	nextIsAnd := false
	// ethAddr filter
	if ethAddr != nil {
		queryStr += "WHERE (tx.from_eth_addr = ? OR tx.to_eth_addr = ?) "
		nextIsAnd = true
		args = append(args, ethAddr, ethAddr)
	} else if bjj != nil { // bjj filter
		queryStr += "WHERE (tx.from_bjj = ? OR tx.to_bjj = ?) "
		nextIsAnd = true
		args = append(args, bjj, bjj)
	}
	// tokenID filter
	if tokenID != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "tx.token_id = ? "
		args = append(args, tokenID)
		nextIsAnd = true
	}
	// idx filter
	if idx != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "(tx.effective_from_idx = ? OR tx.to_idx = ?) "
		args = append(args, idx, idx)
		nextIsAnd = true
	}
	// batchNum filter
	if batchNum != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "tx.batch_num = ? "
		args = append(args, batchNum)
		nextIsAnd = true
	}
	// txType filter
	if txType != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "tx.type = ? "
		args = append(args, txType)
		nextIsAnd = true
	}
	if fromItem != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		if order == OrderAsc {
			queryStr += "tx.item_id >= ? "
		} else {
			queryStr += "tx.item_id <= ? "
		}
		args = append(args, fromItem)
		nextIsAnd = true
	}
	if nextIsAnd {
		queryStr += "AND "
	} else {
		queryStr += "WHERE "
	}
	queryStr += "tx.batch_num IS NOT NULL "

	// pagination
	queryStr += "ORDER BY tx.item_id "
	if order == OrderAsc {
		queryStr += " ASC "
	} else {
		queryStr += " DESC "
	}
	queryStr += fmt.Sprintf("LIMIT %d;", *limit)
	query = hdb.dbRead.Rebind(queryStr)
	// log.Debug(query)
	txsPtrs := []*TxAPI{}
	if err := meddler.QueryAll(hdb.dbRead, &txsPtrs, query, args...); err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	txs := db.SlicePtrsToSlice(txsPtrs).([]TxAPI)
	if len(txs) == 0 {
		return txs, 0, nil
	}
	return txs, txs[0].TotalItems - uint64(len(txs)), nil
}

// GetExitAPI returns a exit from the DB
func (hdb *HistoryDB) GetExitAPI(batchNum *uint, idx *common.Idx) (*ExitAPI, error) {
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	exit := &ExitAPI{}
	err = meddler.QueryRow(
		hdb.dbRead, exit, `SELECT exit_tree.item_id, exit_tree.batch_num,
		hez_idx(exit_tree.account_idx, token.symbol) AS account_idx,
		account.bjj, account.eth_addr,
		exit_tree.merkle_proof, exit_tree.balance, exit_tree.instant_withdrawn,
		exit_tree.delayed_withdraw_request, exit_tree.delayed_withdrawn,
		token.token_id, token.item_id AS token_item_id, 
		token.eth_block_num AS token_block, token.eth_addr AS token_eth_addr, token.name, token.symbol, 
		token.decimals, token.usd, token.usd_update
		FROM exit_tree INNER JOIN account ON exit_tree.account_idx = account.idx 
		INNER JOIN token ON account.token_id = token.token_id 
		WHERE exit_tree.batch_num = $1 AND exit_tree.account_idx = $2;`, batchNum, idx,
	)
	return exit, tracerr.Wrap(err)
}

// GetExitsAPI returns a list of exits from the DB and pagination info
func (hdb *HistoryDB) GetExitsAPI(
	ethAddr *ethCommon.Address, bjj *babyjub.PublicKeyComp, tokenID *common.TokenID,
	idx *common.Idx, batchNum *uint, onlyPendingWithdraws *bool,
	fromItem, limit *uint, order string,
) ([]ExitAPI, uint64, error) {
	if ethAddr != nil && bjj != nil {
		return nil, 0, tracerr.Wrap(errors.New("ethAddr and bjj are incompatible"))
	}
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	var query string
	var args []interface{}
	queryStr := `SELECT exit_tree.item_id, exit_tree.batch_num,
	hez_idx(exit_tree.account_idx, token.symbol) AS account_idx,
	account.bjj, account.eth_addr,
	exit_tree.merkle_proof, exit_tree.balance, exit_tree.instant_withdrawn,
	exit_tree.delayed_withdraw_request, exit_tree.delayed_withdrawn,
	token.token_id, token.item_id AS token_item_id,
	token.eth_block_num AS token_block, token.eth_addr AS token_eth_addr, token.name, token.symbol,
	token.decimals, token.usd, token.usd_update, COUNT(*) OVER() AS total_items
	FROM exit_tree INNER JOIN account ON exit_tree.account_idx = account.idx 
	INNER JOIN token ON account.token_id = token.token_id `
	// Apply filters
	nextIsAnd := false
	// ethAddr filter
	if ethAddr != nil {
		queryStr += "WHERE account.eth_addr = ? "
		nextIsAnd = true
		args = append(args, ethAddr)
	} else if bjj != nil { // bjj filter
		queryStr += "WHERE account.bjj = ? "
		nextIsAnd = true
		args = append(args, bjj)
	}
	// tokenID filter
	if tokenID != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "account.token_id = ? "
		args = append(args, tokenID)
		nextIsAnd = true
	}
	// idx filter
	if idx != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "exit_tree.account_idx = ? "
		args = append(args, idx)
		nextIsAnd = true
	}
	// batchNum filter
	if batchNum != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "exit_tree.batch_num = ? "
		args = append(args, batchNum)
		nextIsAnd = true
	}
	// onlyPendingWithdraws
	if onlyPendingWithdraws != nil {
		if *onlyPendingWithdraws {
			if nextIsAnd {
				queryStr += "AND "
			} else {
				queryStr += "WHERE "
			}
			queryStr += "(exit_tree.instant_withdrawn IS NULL AND exit_tree.delayed_withdrawn IS NULL) "
			nextIsAnd = true
		}
	}
	if fromItem != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		if order == OrderAsc {
			queryStr += "exit_tree.item_id >= ? "
		} else {
			queryStr += "exit_tree.item_id <= ? "
		}
		args = append(args, fromItem)
		// nextIsAnd = true
	}
	// pagination
	queryStr += "ORDER BY exit_tree.item_id "
	if order == OrderAsc {
		queryStr += " ASC "
	} else {
		queryStr += " DESC "
	}
	queryStr += fmt.Sprintf("LIMIT %d;", *limit)
	query = hdb.dbRead.Rebind(queryStr)
	// log.Debug(query)
	exits := []*ExitAPI{}
	if err := meddler.QueryAll(hdb.dbRead, &exits, query, args...); err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	if len(exits) == 0 {
		return []ExitAPI{}, 0, nil
	}
	return db.SlicePtrsToSlice(exits).([]ExitAPI), exits[0].TotalItems - uint64(len(exits)), nil
}

// GetCoordinatorsAPI returns a list of coordinators from the DB and pagination info
func (hdb *HistoryDB) GetCoordinatorsAPI(
	bidderAddr, forgerAddr *ethCommon.Address,
	fromItem, limit *uint, order string,
) ([]CoordinatorAPI, uint64, error) {
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	var query string
	var args []interface{}
	queryStr := `SELECT coordinator.*, COUNT(*) OVER() AS total_items
	FROM coordinator INNER JOIN (
		SELECT MAX(item_id) AS item_id FROM coordinator
		GROUP BY bidder_addr
	) c ON coordinator.item_id = c.item_id `
	// Apply filters
	nextIsAnd := false
	if bidderAddr != nil {
		queryStr += "WHERE bidder_addr = ? "
		nextIsAnd = true
		args = append(args, bidderAddr)
	}
	if forgerAddr != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "forger_addr = ? "
		nextIsAnd = true
		args = append(args, forgerAddr)
	}
	if fromItem != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		if order == OrderAsc {
			queryStr += "coordinator.item_id >= ? "
		} else {
			queryStr += "coordinator.item_id <= ? "
		}
		args = append(args, fromItem)
	}
	// pagination
	queryStr += "ORDER BY coordinator.item_id "
	if order == OrderAsc {
		queryStr += " ASC "
	} else {
		queryStr += " DESC "
	}
	queryStr += fmt.Sprintf("LIMIT %d;", *limit)
	query = hdb.dbRead.Rebind(queryStr)

	coordinators := []*CoordinatorAPI{}
	if err := meddler.QueryAll(hdb.dbRead, &coordinators, query, args...); err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	if len(coordinators) == 0 {
		return []CoordinatorAPI{}, 0, nil
	}
	return db.SlicePtrsToSlice(coordinators).([]CoordinatorAPI),
		coordinators[0].TotalItems - uint64(len(coordinators)), nil
}

// GetAuctionVarsAPI returns auction variables
func (hdb *HistoryDB) GetAuctionVarsAPI() (*common.AuctionVariables, error) {
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	auctionVars := &common.AuctionVariables{}
	err = meddler.QueryRow(
		hdb.dbRead, auctionVars, `SELECT * FROM auction_vars;`,
	)
	return auctionVars, tracerr.Wrap(err)
}

// GetAccountAPI returns an account by its index
func (hdb *HistoryDB) GetAccountAPI(idx common.Idx) (*AccountAPI, error) {
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	account := &AccountAPI{}
	err = meddler.QueryRow(hdb.dbRead, account, `SELECT account.item_id, hez_idx(account.idx, 
		token.symbol) as idx, account.batch_num, account.bjj, account.eth_addr,
		token.token_id, token.item_id AS token_item_id, token.eth_block_num AS token_block,
		token.eth_addr as token_eth_addr, token.name, token.symbol, token.decimals, token.usd,
		token.usd_update, account_update.nonce, account_update.balance 
		FROM account inner JOIN (
			SELECT idx, nonce, balance 
			FROM account_update
			WHERE idx = $1
			ORDER BY item_id DESC LIMIT 1
		) AS account_update ON account_update.idx = account.idx 
		INNER JOIN token ON account.token_id = token.token_id
		WHERE account.idx = $1;`, idx)

	if err != nil {
		return nil, tracerr.Wrap(err)
	}

	return account, nil
}

// GetAccountsAPI returns a list of accounts from the DB and pagination info
func (hdb *HistoryDB) GetAccountsAPI(
	tokenIDs []common.TokenID, ethAddr *ethCommon.Address,
	bjj *babyjub.PublicKeyComp, fromItem, limit *uint, order string,
) ([]AccountAPI, uint64, error) {
	if ethAddr != nil && bjj != nil {
		return nil, 0, tracerr.Wrap(errors.New("ethAddr and bjj are incompatible"))
	}
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	var query string
	var args []interface{}
	queryStr := `SELECT account.item_id, hez_idx(account.idx, token.symbol) as idx, account.batch_num, 
	account.bjj, account.eth_addr, token.token_id, token.item_id AS token_item_id, token.eth_block_num AS token_block,
	token.eth_addr as token_eth_addr, token.name, token.symbol, token.decimals, token.usd, token.usd_update, 
	account_update.nonce, account_update.balance, COUNT(*) OVER() AS total_items
	FROM account inner JOIN (
		SELECT DISTINCT idx,
		first_value(nonce) over(partition by idx ORDER BY item_id DESC) as nonce,
		first_value(balance) over(partition by idx ORDER BY item_id DESC) as balance
		FROM account_update
	) AS account_update ON account_update.idx = account.idx INNER JOIN token ON account.token_id = token.token_id `
	// Apply filters
	nextIsAnd := false
	// ethAddr filter
	if ethAddr != nil {
		queryStr += "WHERE account.eth_addr = ? "
		nextIsAnd = true
		args = append(args, ethAddr)
	} else if bjj != nil { // bjj filter
		queryStr += "WHERE account.bjj = ? "
		nextIsAnd = true
		args = append(args, bjj)
	}
	// tokenID filter
	if len(tokenIDs) > 0 {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "account.token_id IN (?) "
		args = append(args, tokenIDs)
		nextIsAnd = true
	}
	if fromItem != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		if order == OrderAsc {
			queryStr += "account.item_id >= ? "
		} else {
			queryStr += "account.item_id <= ? "
		}
		args = append(args, fromItem)
	}
	// pagination
	queryStr += "ORDER BY account.item_id "
	if order == OrderAsc {
		queryStr += " ASC "
	} else {
		queryStr += " DESC "
	}
	queryStr += fmt.Sprintf("LIMIT %d;", *limit)
	query, argsQ, err := sqlx.In(queryStr, args...)
	if err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	query = hdb.dbRead.Rebind(query)

	accounts := []*AccountAPI{}
	if err := meddler.QueryAll(hdb.dbRead, &accounts, query, argsQ...); err != nil {
		return nil, 0, tracerr.Wrap(err)
	}
	if len(accounts) == 0 {
		return []AccountAPI{}, 0, nil
	}

	return db.SlicePtrsToSlice(accounts).([]AccountAPI),
		accounts[0].TotalItems - uint64(len(accounts)), nil
}

// GetCommonAccountAPI returns the account associated to an account idx
func (hdb *HistoryDB) GetCommonAccountAPI(idx common.Idx) (*common.Account, error) {
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	account := &common.Account{}
	err = meddler.QueryRow(
		hdb.dbRead, account, `SELECT idx, token_id, batch_num, bjj, eth_addr
		FROM account WHERE idx = $1;`, idx,
	)
	return account, tracerr.Wrap(err)
}

// GetCoordinatorAPI returns a coordinator by its bidderAddr
func (hdb *HistoryDB) GetCoordinatorAPI(bidderAddr ethCommon.Address) (*CoordinatorAPI, error) {
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	return hdb.getCoordinatorAPI(hdb.dbRead, bidderAddr)
}

func (hdb *HistoryDB) getCoordinatorAPI(d meddler.DB, bidderAddr ethCommon.Address) (*CoordinatorAPI, error) {
	coordinator := &CoordinatorAPI{}
	err := meddler.QueryRow(
		d, coordinator,
		"SELECT * FROM coordinator WHERE bidder_addr = $1 ORDER BY item_id DESC LIMIT 1;",
		bidderAddr,
	)
	return coordinator, tracerr.Wrap(err)
}

// GetNodeInfoAPI retusnt he NodeInfo
func (hdb *HistoryDB) GetNodeInfoAPI() (*NodeInfo, error) {
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	return hdb.GetNodeInfo()
}

// GetBucketUpdatesInternalAPI returns the latest bucket updates
func (hdb *HistoryDB) GetBucketUpdatesInternalAPI() ([]BucketUpdateAPI, error) {
	var bucketUpdates []*BucketUpdateAPI
	err := meddler.QueryAll(
		hdb.dbRead, &bucketUpdates,
		`SELECT num_bucket, withdrawals FROM bucket_update 
			WHERE item_id in(SELECT max(item_id) FROM bucket_update 
			group by num_bucket) 
			ORDER BY num_bucket ASC;`,
	)
	return db.SlicePtrsToSlice(bucketUpdates).([]BucketUpdateAPI), tracerr.Wrap(err)
}

// GetNextForgersInternalAPI returns next forgers
func (hdb *HistoryDB) GetNextForgersInternalAPI(auctionVars *common.AuctionVariables,
	auctionConsts *common.AuctionConstants,
	lastBlock common.Block, currentSlot, lastClosedSlot int64) ([]NextForgerAPI, error) {
	secondsPerBlock := int64(15) //nolint:gomnd
	// currentSlot and lastClosedSlot included
	limit := uint(lastClosedSlot - currentSlot + 1)
	bids, _, err := hdb.getBestBidsAPI(hdb.dbRead, &currentSlot, &lastClosedSlot, nil, &limit, "ASC")
	if err != nil && tracerr.Unwrap(err) != sql.ErrNoRows {
		return nil, tracerr.Wrap(err)
	}
	nextForgers := []NextForgerAPI{}
	// Get min bid info
	var minBidInfo []MinBidInfo
	if currentSlot >= auctionVars.DefaultSlotSetBidSlotNum {
		// All min bids can be calculated with the last update of AuctionVariables

		minBidInfo = []MinBidInfo{{
			DefaultSlotSetBid:        auctionVars.DefaultSlotSetBid,
			DefaultSlotSetBidSlotNum: auctionVars.DefaultSlotSetBidSlotNum,
		}}
	} else {
		// Get all the relevant updates from the DB
		minBidInfo, err = hdb.getMinBidInfo(hdb.dbRead, currentSlot, lastClosedSlot)
		if err != nil {
			return nil, tracerr.Wrap(err)
		}
	}
	// Create nextForger for each slot
	for i := currentSlot; i <= lastClosedSlot; i++ {
		fromBlock := i*int64(auctionConsts.BlocksPerSlot) +
			auctionConsts.GenesisBlockNum
		toBlock := (i+1)*int64(auctionConsts.BlocksPerSlot) +
			auctionConsts.GenesisBlockNum - 1
		nextForger := NextForgerAPI{
			Period: Period{
				SlotNum:   i,
				FromBlock: fromBlock,
				ToBlock:   toBlock,
				FromTimestamp: lastBlock.Timestamp.Add(time.Second *
					time.Duration(secondsPerBlock*(fromBlock-lastBlock.Num))),
				ToTimestamp: lastBlock.Timestamp.Add(time.Second *
					time.Duration(secondsPerBlock*(toBlock-lastBlock.Num))),
			},
		}
		foundForger := false
		// If there is a bid for a slot, get forger (coordinator)
		for j := range bids {
			slotNum := bids[j].SlotNum
			if slotNum == i {
				// There's a bid for the slot
				// Check if the bid is greater than the minimum required
				for i := 0; i < len(minBidInfo); i++ {
					// Find the most recent update
					if slotNum >= minBidInfo[i].DefaultSlotSetBidSlotNum {
						// Get min bid
						minBidSelector := slotNum % int64(len(auctionVars.DefaultSlotSetBid))
						minBid := minBidInfo[i].DefaultSlotSetBid[minBidSelector]
						// Check if the bid has beaten the minimum
						bid, ok := new(big.Int).SetString(string(bids[j].BidValue), 10)
						if !ok {
							return nil, tracerr.New("Wrong bid value, error parsing it as big.Int")
						}
						if minBid.Cmp(bid) == 1 {
							// Min bid is greater than bid, the slot will be forged by boot coordinator
							break
						}
						foundForger = true
						break
					}
				}
				if !foundForger { // There is no bid or it's smaller than the minimum
					break
				}
				coordinator, err := hdb.getCoordinatorAPI(hdb.dbRead, bids[j].Bidder)
				if err != nil {
					return nil, tracerr.Wrap(err)
				}
				nextForger.Coordinator = *coordinator
				break
			}
		}
		// If there is no bid, the coordinator that will forge is boot coordinator
		if !foundForger {
			nextForger.Coordinator = CoordinatorAPI{
				Forger: auctionVars.BootCoordinator,
				URL:    auctionVars.BootCoordinatorURL,
			}
		}
		nextForgers = append(nextForgers, nextForger)
	}
	return nextForgers, nil
}

// GetMetricsInternalAPI returns the MetricsAPI
func (hdb *HistoryDB) GetMetricsInternalAPI(lastBatchNum common.BatchNum) (*MetricsAPI, error) {
	var metrics MetricsAPI
	// Get the first and last batch of the last 24h and their timestamps
	// if u.state.Network.LastBatch == nil {
	// 	return &metrics, nil
	// }
	type period struct {
		FromBatchNum  common.BatchNum `meddler:"from_batch_num"`
		FromTimestamp time.Time       `meddler:"from_timestamp"`
		ToBatchNum    common.BatchNum `meddler:"-"`
		ToTimestamp   time.Time       `meddler:"to_timestamp"`
	}
	p := &period{
		ToBatchNum: lastBatchNum,
	}
	if err := meddler.QueryRow(
		hdb.dbRead, p, `SELECT
			COALESCE (MIN(batch.batch_num), 0) as from_batch_num,
			COALESCE (MIN(block.timestamp), NOW()) AS from_timestamp, 
			COALESCE (MAX(block.timestamp), NOW()) AS to_timestamp
			FROM batch INNER JOIN block ON batch.eth_block_num = block.eth_block_num
			WHERE block.timestamp >= NOW() - INTERVAL '24 HOURS';`,
	); err != nil {
		return nil, tracerr.Wrap(err)
	}
	// Get the amount of txs of that period
	row := hdb.dbRead.QueryRow(
		`SELECT COUNT(*) as total_txs FROM tx WHERE tx.batch_num between $1 AND $2;`,
		p.FromBatchNum, p.ToBatchNum,
	)
	var nTxs int
	if err := row.Scan(&nTxs); err != nil {
		return nil, tracerr.Wrap(err)
	}
	// Set txs/s
	seconds := p.ToTimestamp.Sub(p.FromTimestamp).Seconds()
	if seconds == 0 { // Avoid dividing by 0
		seconds++
	}
	metrics.TransactionsPerSecond = float64(nTxs) / seconds
	// Set txs/batch
	nBatches := p.ToBatchNum - p.FromBatchNum + 1
	if nBatches == 0 { // Avoid dividing by 0
		nBatches++
	}
	if (p.ToBatchNum - p.FromBatchNum) > 0 {
		metrics.TransactionsPerBatch = float64(nTxs) /
			float64(nBatches)
	} else {
		metrics.TransactionsPerBatch = 0
	}
	// Get total fee of that period
	row = hdb.dbRead.QueryRow(
		`SELECT COALESCE (SUM(total_fees_usd), 0) FROM batch WHERE batch_num between $1 AND $2;`,
		p.FromBatchNum, p.ToBatchNum,
	)
	var totalFee float64
	if err := row.Scan(&totalFee); err != nil {
		return nil, tracerr.Wrap(err)
	}
	// Set batch frequency
	metrics.BatchFrequency = seconds / float64(nBatches)
	// Set avg transaction fee (only L2 txs have fee)
	row = hdb.dbRead.QueryRow(
		`SELECT COUNT(*) as total_txs FROM tx WHERE tx.batch_num between $1 AND $2 AND NOT is_l1;`,
		p.FromBatchNum, p.ToBatchNum,
	)
	var nL2Txs int
	if err := row.Scan(&nL2Txs); err != nil {
		return nil, tracerr.Wrap(err)
	}
	if nL2Txs > 0 {
		metrics.AvgTransactionFee = totalFee / float64(nL2Txs)
	} else {
		metrics.AvgTransactionFee = 0
	}
	// Get and set amount of registered accounts
	type registeredAccounts struct {
		TotalIdx int64 `meddler:"total_idx"`
		TotalBJJ int64 `meddler:"total_bjj"`
	}
	ra := &registeredAccounts{}
	if err := meddler.QueryRow(
		hdb.dbRead, ra,
		`SELECT COUNT(*) AS total_bjj, COUNT(DISTINCT(bjj)) AS total_idx FROM account;`,
	); err != nil {
		return nil, tracerr.Wrap(err)
	}
	metrics.TotalAccounts = ra.TotalIdx
	metrics.TotalBJJs = ra.TotalBJJ
	// Get and set estimated time to forge L1 tx
	row = hdb.dbRead.QueryRow(
		`SELECT COALESCE (AVG(EXTRACT(EPOCH FROM (forged.timestamp - added.timestamp))), 0) FROM tx
			INNER JOIN block AS added ON tx.eth_block_num = added.eth_block_num
			INNER JOIN batch AS forged_batch ON tx.batch_num = forged_batch.batch_num
			INNER JOIN block AS forged ON forged_batch.eth_block_num = forged.eth_block_num
			WHERE tx.batch_num between $1 and $2 AND tx.is_l1 AND tx.user_origin;`,
		p.FromBatchNum, p.ToBatchNum,
	)
	var timeToForgeL1 float64
	if err := row.Scan(&timeToForgeL1); err != nil {
		return nil, tracerr.Wrap(err)
	}
	metrics.EstimatedTimeToForgeL1 = timeToForgeL1
	return &metrics, nil
}

// GetStateAPI returns the StateAPI
func (hdb *HistoryDB) GetStateAPI() (*StateAPI, error) {
	cancel, err := hdb.apiConnCon.Acquire()
	defer cancel()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	defer hdb.apiConnCon.Release()
	return hdb.getStateAPI(hdb.dbRead)
}

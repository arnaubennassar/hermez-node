package common

import (
	"math/big"
	"testing"

	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/iden3/go-iden3-crypto/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewL1Tx(t *testing.T) {
	toForge := new(int64)
	*toForge = 123456
	fromIdx := new(Idx)
	*fromIdx = 300
	l1Tx := &L1Tx{
		ToForgeL1TxsNum: toForge,
		Position:        71,
		ToIdx:           301,
		TokenID:         5,
		Amount:          big.NewInt(1),
		LoadAmount:      big.NewInt(2),
		FromIdx:         fromIdx,
	}
	l1Tx, err := NewL1Tx(l1Tx)
	assert.Nil(t, err)
	assert.Equal(t, "0x01000000000001e240004700", l1Tx.TxID.String())
}

func TestL1TxByteParsers(t *testing.T) {
	var pkComp babyjub.PublicKeyComp
	err := pkComp.UnmarshalText([]byte("0x56ca90f80d7c374ae7485e9bcc47d4ac399460948da6aeeb899311097925a72c"))
	require.Nil(t, err)

	pk, err := pkComp.Decompress()
	require.Nil(t, err)

	fromIdx := new(Idx)
	*fromIdx = 2
	l1Tx := &L1Tx{
		ToIdx:       3,
		TokenID:     5,
		Amount:      big.NewInt(1),
		LoadAmount:  big.NewInt(2),
		FromIdx:     fromIdx,
		FromBJJ:     pk,
		FromEthAddr: ethCommon.HexToAddress("0xc58d29fA6e86E4FAe04DDcEd660d45BCf3Cb2370"),
	}

	expected, err := utils.HexDecode("c58d29fa6e86e4fae04ddced660d45bcf3cb237056ca90f80d7c374ae7485e9bcc47d4ac399460948da6aeeb899311097925a72c0000000000020002000100000005000000000003")
	require.Nil(t, err)

	encodedData, err := l1Tx.Bytes(32)
	require.Nil(t, err)
	assert.Equal(t, expected, encodedData)

	decodedData, err := L1TxFromBytes(encodedData)
	require.Nil(t, err)
	assert.Equal(t, l1Tx, decodedData)

	encodedData2, err := decodedData.Bytes(32)
	require.Nil(t, err)
	assert.Equal(t, encodedData, encodedData2)

	// expect error if length!=68
	_, err = L1TxFromBytes(encodedData[:66])
	require.NotNil(t, err)
	_, err = L1TxFromBytes([]byte{})
	require.NotNil(t, err)
	_, err = L1TxFromBytes(nil)
	require.NotNil(t, err)
}

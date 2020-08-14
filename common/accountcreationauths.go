package common

import (
	"time"

	eth "github.com/ethereum/go-ethereum/common"
	"github.com/iden3/go-iden3-crypto/babyjub"
)

// AccountCreationAuth authorizations sent by users to the L2DB, to be used for account creations when necessary
type AccountCreationAuth struct {
	Timestamp time.Time
	EthAddr   eth.Address
	BJJ       babyjub.PublicKey
	Signature babyjub.Signature
}
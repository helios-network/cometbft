package coregrpc

import (
	"github.com/tendermint/tendermint/crypto"
)

const traceIDLen int = 6

func newTraceID() string {
	return crypto.CRandHex(traceIDLen)
}

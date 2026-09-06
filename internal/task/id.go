package task

import (
	"math/big"
	"math/rand/v2"
	"time"
)

// rollRange is the size of the random component of a task ID.
const rollRange = 4_000_000_000

// NewID returns an identifier for a new task: the current time in
// milliseconds with a random tail, which is unique enough in practice and
// keeps IDs roughly ordered by creation.
func NewID() *big.Int {
	id := big.NewInt(time.Now().UnixMilli())
	id.Mul(id, big.NewInt(rollRange))
	return id.Add(id, big.NewInt(rand.Int64N(rollRange)))
}

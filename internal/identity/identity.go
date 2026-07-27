package identity

import (
	"fmt"
	"sync/atomic"
	"time"
)

var sequence atomic.Uint64

func New(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), sequence.Add(1))
}

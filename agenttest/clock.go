package agenttest

import (
	"sync"
	"time"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

// Clock 返回每次按固定步长前进的并发安全时钟。
func Clock(start time.Time, step time.Duration) agent.Clock {
	var mu sync.Mutex
	current := start
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		value := current
		current = current.Add(step)
		return value
	}
}

package retry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestRandomDelayInRange verifies random delays stay within configured bounds.
func TestRandomDelayInRange(t *testing.T) {
	t.Parallel()

	const (
		minDelay = 10 * time.Millisecond
		maxDelay = 20 * time.Millisecond
	)

	policy := &randomRangePolicy{
		minDelay: minDelay,
		maxDelay: maxDelay,
	}

	for range 500 {
		delay := policy.randomDelayInRange()
		assert.GreaterOrEqual(t, delay, minDelay)
		assert.Less(t, delay, maxDelay)
	}
}

// TestRandomDelayInRange_SwapsBounds verifies reversed bounds are normalized before sampling.
func TestRandomDelayInRange_SwapsBounds(t *testing.T) {
	t.Parallel()

	const (
		minDelay = 5 * time.Millisecond
		maxDelay = 15 * time.Millisecond
	)

	policy := &randomRangePolicy{
		minDelay: maxDelay,
		maxDelay: minDelay,
	}

	for range 500 {
		delay := policy.randomDelayInRange()
		assert.GreaterOrEqual(t, delay, minDelay)
		assert.Less(t, delay, maxDelay)
	}
}

// TestRandomDelayInRange_EqualBounds verifies equal bounds return the exact configured delay.
func TestRandomDelayInRange_EqualBounds(t *testing.T) {
	t.Parallel()

	expectedDelay := 42 * time.Millisecond
	policy := &randomRangePolicy{
		minDelay: expectedDelay,
		maxDelay: expectedDelay,
	}
	assert.Equal(t, expectedDelay, policy.randomDelayInRange())
}

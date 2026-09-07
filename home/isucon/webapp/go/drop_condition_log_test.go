package main

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestNoteDroppedIsuConditionCounts(t *testing.T) {
	atomic.StoreUint64(&dropConditionPending, 0)
	atomic.StoreUint64(&dropConditionTotal, 0)

	noteDroppedIsuCondition()
	noteDroppedIsuCondition()
	noteDroppedIsuCondition()

	if got := atomic.LoadUint64(&dropConditionPending); got != 3 {
		t.Fatalf("pending=%d want 3", got)
	}
	if got := atomic.LoadUint64(&dropConditionTotal); got != 3 {
		t.Fatalf("total=%d want 3", got)
	}
	if n := atomic.SwapUint64(&dropConditionPending, 0); n != 3 {
		t.Fatalf("flush pending=%d want 3", n)
	}
	if got := atomic.LoadUint64(&dropConditionPending); got != 0 {
		t.Fatalf("pending after flush=%d want 0", got)
	}
	if got := atomic.LoadUint64(&dropConditionTotal); got != 3 {
		t.Fatalf("total after flush=%d want 3", got)
	}
}

func TestDropConditionLogIdleDuration(t *testing.T) {
	if dropConditionLogIdle < 5*time.Second {
		t.Fatalf("idle=%s; load overlap を避けるには数秒以上必要", dropConditionLogIdle)
	}
}

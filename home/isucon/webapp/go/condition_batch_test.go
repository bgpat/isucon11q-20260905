package main

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestConditionBatcher(flushFn func(args []interface{}, rowCount int) error) *conditionBatcher {
	return &conditionBatcher{
		collectWindow:  20 * time.Millisecond,
		responseBudget: 80 * time.Millisecond,
		insertReserve:  30 * time.Millisecond,
		maxRows:        500,
		flushFn:        flushFn,
	}
}

func TestConditionBatcherAggregatesWithinWindow(t *testing.T) {
	var flushCount int32
	var flushedRows int32
	batcher := newTestConditionBatcher(func(args []interface{}, rowCount int) error {
		atomic.AddInt32(&flushCount, 1)
		atomic.AddInt32(&flushedRows, int32(rowCount))
		if len(args) != rowCount*5 {
			t.Errorf("args len=%d want %d", len(args), rowCount*5)
		}
		return nil
	})

	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := []interface{}{"u", time.Unix(int64(i), 0), false, "is_dirty=false,is_overweight=false,is_broken=false", "m"}
			if err := batcher.enqueue(args, 1, start); err != nil {
				t.Errorf("enqueue: %v", err)
			}
		}(i)
	}
	wg.Wait()

	if got := atomic.LoadInt32(&flushCount); got != 1 {
		t.Fatalf("flushCount=%d want 1", got)
	}
	if got := atomic.LoadInt32(&flushedRows); got != 3 {
		t.Fatalf("flushedRows=%d want 3", got)
	}
}

func TestConditionBatcherFlushesAtMaxRows(t *testing.T) {
	var flushCount int32
	batcher := newTestConditionBatcher(func(args []interface{}, rowCount int) error {
		atomic.AddInt32(&flushCount, 1)
		return nil
	})
	batcher.collectWindow = time.Hour
	batcher.maxRows = 2

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := []interface{}{"u", time.Unix(int64(i), 0), false, "is_dirty=false,is_overweight=false,is_broken=false", "m"}
			if err := batcher.enqueue(args, 1, start); err != nil {
				t.Errorf("enqueue: %v", err)
			}
		}(i)
	}
	wg.Wait()

	if got := atomic.LoadInt32(&flushCount); got != 1 {
		t.Fatalf("flushCount=%d want 1", got)
	}
}

func TestConditionBatcherPropagatesFlushError(t *testing.T) {
	wantErr := errors.New("flush failed")
	batcher := newTestConditionBatcher(func(args []interface{}, rowCount int) error {
		return wantErr
	})
	batcher.collectWindow = 0

	err := batcher.enqueue(
		[]interface{}{"u", time.Unix(1, 0), false, "is_dirty=false,is_overweight=false,is_broken=false", "m"},
		1,
		time.Now(),
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err=%v want %v", err, wantErr)
	}
}

func TestConditionBatcherAcceptsNextBatchWhileFlushing(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var flushCount int32
	batcher := newTestConditionBatcher(func(args []interface{}, rowCount int) error {
		n := atomic.AddInt32(&flushCount, 1)
		if n == 1 {
			close(started)
			<-release
		}
		return nil
	})
	batcher.collectWindow = 0
	batcher.maxRows = 1

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = batcher.enqueue(
			[]interface{}{"u1", time.Unix(1, 0), false, "is_dirty=false,is_overweight=false,is_broken=false", "m"},
			1,
			time.Now(),
		)
	}()

	<-started

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = batcher.enqueue(
			[]interface{}{"u2", time.Unix(2, 0), false, "is_dirty=false,is_overweight=false,is_broken=false", "m"},
			1,
			time.Now(),
		)
	}()

	// 最初の flush が終わる前に次が受け付けられていること（open が取れること）を少し待つ。
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&flushCount); got != 2 {
		t.Fatalf("flushCount=%d want 2", got)
	}
}

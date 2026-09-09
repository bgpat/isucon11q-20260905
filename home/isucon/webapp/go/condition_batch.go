package main

import (
	"fmt"
	"sync"
	"time"
)

const (
	conditionCollectWindow  = 50 * time.Millisecond
	conditionResponseBudget = 80 * time.Millisecond
	conditionInsertReserve  = 30 * time.Millisecond
	conditionBatchMaxRows   = 500
)

type conditionBatch struct {
	args     []interface{}
	rowCount int
	waiters  []chan error
	openedAt time.Time
}

// conditionBatcher は短い待ち窓で複数リクエストの行を 1 INSERT にまとめる。
// classic singleflight（同一 key の重複排除）ではなく、時間窓の行バッファである。
type conditionBatcher struct {
	mu             sync.Mutex
	open           *conditionBatch
	timer          *time.Timer
	collectWindow  time.Duration
	responseBudget time.Duration
	insertReserve  time.Duration
	maxRows        int
	flushFn        func(args []interface{}, rowCount int) error
}

var globalConditionBatcher = &conditionBatcher{
	collectWindow:  conditionCollectWindow,
	responseBudget: conditionResponseBudget,
	insertReserve:  conditionInsertReserve,
	maxRows:        conditionBatchMaxRows,
	flushFn:        flushIsuConditionRows,
}

func flushIsuConditionRows(args []interface{}, rowCount int) error {
	result, err := db.Exec(
		"INSERT INTO `isu_condition` (`jia_isu_uuid`, `timestamp`, `is_sitting`, `condition`, `message`) VALUES "+
			buildIsuConditionInsertValues(rowCount),
		args...,
	)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted != int64(rowCount) {
		return fmt.Errorf("unexpected inserted condition count: got=%d want=%d", inserted, rowCount)
	}
	markTrendMemoryNotReady()
	invalidateGraphCacheFromInsertArgs(args)
	return nil
}

func (b *conditionBatcher) enqueue(args []interface{}, rowCount int, requestStart time.Time) error {
	done := make(chan error, 1)

	b.mu.Lock()
	if b.open == nil {
		b.open = &conditionBatch{openedAt: time.Now()}
		wait := b.collectWindow
		deadline := requestStart.Add(b.responseBudget - b.insertReserve)
		if until := time.Until(deadline); until < wait {
			wait = until
		}
		if wait > 0 {
			b.timer = time.AfterFunc(wait, b.onTimer)
		}
	}

	batch := b.open
	batch.args = append(batch.args, args...)
	batch.rowCount += rowCount
	batch.waiters = append(batch.waiters, done)

	shouldFlush := batch.rowCount >= b.maxRows || b.timer == nil
	deadline := requestStart.Add(b.responseBudget - b.insertReserve)
	if time.Until(deadline) <= 0 {
		shouldFlush = true
	}

	var toFlush *conditionBatch
	if shouldFlush {
		toFlush = b.takeOpenLocked()
	}
	b.mu.Unlock()

	if toFlush != nil {
		go b.runFlush(toFlush)
	}
	return <-done
}

func (b *conditionBatcher) takeOpenLocked() *conditionBatch {
	batch := b.open
	b.open = nil
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	return batch
}

func (b *conditionBatcher) onTimer() {
	b.mu.Lock()
	batch := b.takeOpenLocked()
	b.mu.Unlock()
	if batch != nil && batch.rowCount > 0 {
		b.runFlush(batch)
	}
}

func (b *conditionBatcher) runFlush(batch *conditionBatch) {
	var err error
	if batch.rowCount > 0 && b.flushFn != nil {
		err = b.flushFn(batch.args, batch.rowCount)
	}
	for _, w := range batch.waiters {
		w <- err
	}
}

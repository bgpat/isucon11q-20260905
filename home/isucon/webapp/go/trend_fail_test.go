package main

import (
	"testing"
	"time"
)

func TestShouldFailTrendAfterDelayThenBudget(t *testing.T) {
	origAfter, origBudget := trendFailAfter, trendFailBudget
	t.Cleanup(func() {
		trendFailAfter = origAfter
		trendFailBudget = origBudget
		trendFailAtNano.Store(0)
		trendFailLeft.Store(0)
	})

	trendFailAfter = time.Second
	trendFailBudget = 2
	start := time.Unix(100, 0)
	resetTrendFail(start)

	if shouldFailTrend(start.Add(time.Second - time.Nanosecond)) {
		t.Fatal("before delay should be 200")
	}
	if !shouldFailTrend(start.Add(time.Second)) {
		t.Fatal("1st request after delay should be 500")
	}
	if !shouldFailTrend(start.Add(2 * time.Second)) {
		t.Fatal("2nd request after delay should be 500")
	}
	if shouldFailTrend(start.Add(3 * time.Second)) {
		t.Fatal("budget exhausted should be 200")
	}
}

func TestShouldFailTrendDisabledAndBeforeInitialize(t *testing.T) {
	origAfter, origBudget := trendFailAfter, trendFailBudget
	t.Cleanup(func() {
		trendFailAfter = origAfter
		trendFailBudget = origBudget
		trendFailAtNano.Store(0)
		trendFailLeft.Store(0)
	})

	trendFailAfter = time.Second
	trendFailBudget = 0
	resetTrendFail(time.Unix(1, 0))
	if shouldFailTrend(time.Unix(10, 0)) {
		t.Fatal("count 0 should disable")
	}

	trendFailBudget = 3
	trendFailAtNano.Store(0)
	trendFailLeft.Store(3)
	if shouldFailTrend(time.Unix(10, 0)) {
		t.Fatal("before initialize should be 200")
	}
}

func TestResetTrendFailRestoresBudget(t *testing.T) {
	origAfter, origBudget := trendFailAfter, trendFailBudget
	t.Cleanup(func() {
		trendFailAfter = origAfter
		trendFailBudget = origBudget
		trendFailAtNano.Store(0)
		trendFailLeft.Store(0)
	})

	trendFailAfter = 0
	trendFailBudget = 1
	start := time.Unix(50, 0)
	resetTrendFail(start)
	if !shouldFailTrend(start) {
		t.Fatal("expected 500")
	}
	resetTrendFail(start.Add(time.Second))
	if !shouldFailTrend(start.Add(time.Second)) {
		t.Fatal("initialize should restore budget")
	}
}

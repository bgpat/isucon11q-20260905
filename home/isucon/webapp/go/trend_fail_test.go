package main

import (
	"testing"
	"time"
)

func restoreTrendFail(after, dur time.Duration, budget int64) {
	trendFailAfter = after
	trendFailFor = dur
	trendFailBudget = budget
	trendFailAtNano.Store(0)
	trendFailLeft.Store(0)
}

func TestShouldFailTrendAfterDelayThenBudget(t *testing.T) {
	origAfter, origFor, origBudget := trendFailAfter, trendFailFor, trendFailBudget
	t.Cleanup(func() { restoreTrendFail(origAfter, origFor, origBudget) })

	trendFailAfter = time.Second
	trendFailFor = 0
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

func TestShouldFailTrendWindowThen200(t *testing.T) {
	origAfter, origFor, origBudget := trendFailAfter, trendFailFor, trendFailBudget
	t.Cleanup(func() { restoreTrendFail(origAfter, origFor, origBudget) })

	trendFailAfter = 5 * time.Second
	trendFailFor = 5 * time.Second
	trendFailBudget = 45
	start := time.Unix(0, 0)
	resetTrendFail(start)

	if shouldFailTrend(start.Add(5*time.Second - time.Nanosecond)) {
		t.Fatal("before window should be 200")
	}
	if !shouldFailTrend(start.Add(5 * time.Second)) {
		t.Fatal("start of window should be 500")
	}
	if !shouldFailTrend(start.Add(10*time.Second - time.Nanosecond)) {
		t.Fatal("end of window should be 500")
	}
	if shouldFailTrend(start.Add(10 * time.Second)) {
		t.Fatal("after window should be 200")
	}
}

func TestShouldFailTrendDisabledAndBeforeInitialize(t *testing.T) {
	origAfter, origFor, origBudget := trendFailAfter, trendFailFor, trendFailBudget
	t.Cleanup(func() { restoreTrendFail(origAfter, origFor, origBudget) })

	trendFailAfter = time.Second
	trendFailFor = time.Second
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
	origAfter, origFor, origBudget := trendFailAfter, trendFailFor, trendFailBudget
	t.Cleanup(func() { restoreTrendFail(origAfter, origFor, origBudget) })

	trendFailAfter = 0
	trendFailFor = 0
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

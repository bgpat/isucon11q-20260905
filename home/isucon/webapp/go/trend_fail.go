package main

import (
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	defaultTrendFailAfterSec = 5.0
	defaultTrendFailCount    = 1
)

var (
	trendFailAfter  time.Duration
	trendFailBudget int64
	trendFailAtNano atomic.Int64
	trendFailLeft   atomic.Int64
)

func init() {
	trendFailAfter = durationFromEnvSec("TREND_FAIL_AFTER_SEC", defaultTrendFailAfterSec)
	trendFailBudget = int64FromEnv("TREND_FAIL_COUNT", defaultTrendFailCount)
}

func durationFromEnvSec(key string, fallback float64) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return time.Duration(fallback * float64(time.Second))
	}
	sec, err := strconv.ParseFloat(raw, 64)
	if err != nil || sec < 0 {
		return time.Duration(fallback * float64(time.Second))
	}
	return time.Duration(sec * float64(time.Second))
}

func int64FromEnv(key string, fallback int64) int64 {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func resetTrendFail(now time.Time) {
	trendFailAtNano.Store(now.UnixNano())
	trendFailLeft.Store(trendFailBudget)
}

func shouldFailTrend(now time.Time) bool {
	if trendFailBudget <= 0 {
		return false
	}
	at := trendFailAtNano.Load()
	if at == 0 {
		return false
	}
	if now.UnixNano()-at < int64(trendFailAfter) {
		return false
	}
	for {
		left := trendFailLeft.Load()
		if left <= 0 {
			return false
		}
		if trendFailLeft.CompareAndSwap(left, left-1) {
			return true
		}
	}
}

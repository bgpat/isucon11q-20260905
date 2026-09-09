package main

import (
	"context"
	"sync"

	"golang.org/x/sync/singleflight"
)

type trendFlightHolder struct {
	mu sync.Mutex
	g  singleflight.Group
}

var trendFlight trendFlightHolder

func resetTrendFlight() {
	trendFlight.mu.Lock()
	trendFlight.g = singleflight.Group{}
	trendFlight.mu.Unlock()
}

func computeTrendShared(ctx context.Context) ([]TrendResponse, error) {
	trendFlight.mu.Lock()
	g := trendFlight.g
	trendFlight.mu.Unlock()

	v, err, _ := g.Do("trend", func() (interface{}, error) {
		if res, ok := loadTrendCache(); ok {
			return res, nil
		}
		gen, res, hit := snapshotTrendCache()
		if hit {
			return res, nil
		}
		built, err := buildTrendResponseFromDBValkey(ctx)
		if err != nil {
			return nil, err
		}
		storeTrendCache(gen, built)
		return built, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]TrendResponse), nil
}

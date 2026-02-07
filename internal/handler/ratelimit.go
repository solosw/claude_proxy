package handler

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// modelLimiters 按模型 ID 的 QPS 限流器；MaxQPS 变更时重建对应 limiter。
var (
	modelLimitersMu sync.Mutex
	modelLimiters   = make(map[string]*modelLimiterEntry)
)

type modelLimiterEntry struct {
	limiter *rate.Limiter
	qps     float64
}

// waitModelQPS 若该模型配置了 MaxQPS > 0，则阻塞直到获得令牌或 ctx 取消。
func waitModelQPS(ctx context.Context, modelID string, maxQPS float64) {
	if maxQPS <= 0 {
		return
	}
	modelLimitersMu.Lock()
	entry, ok := modelLimiters[modelID]
	if !ok || entry.qps != maxQPS {
		entry = &modelLimiterEntry{
			limiter: rate.NewLimiter(rate.Limit(maxQPS), 1),
			qps:     maxQPS,
		}
		modelLimiters[modelID] = entry
	}
	lim := entry.limiter
	modelLimitersMu.Unlock()
	_ = lim.Wait(ctx)
}

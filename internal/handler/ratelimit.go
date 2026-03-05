package handler

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// modelLimiters 按模型 ID 的 QPS 限流器；MaxQPS 变更时重建对应 limiter。
var (
	modelLimitersMu sync.Mutex
	modelLimiters   = make(map[string]*modelLimiterEntry)
)

type modelLimiterEntry struct {
	limiter  *rate.Limiter
	qps      float64
	lastUsed time.Time
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
	entry.lastUsed = time.Now()
	lim := entry.limiter
	modelLimitersMu.Unlock()
	_ = lim.Wait(ctx)
}

// cleanupLimiters 定期清理超过 1 小时未使用的 limiter 以防内存泄漏。
func init() {
	go func() {
		for {
			time.Sleep(10 * time.Minute)
			modelLimitersMu.Lock()
			now := time.Now()
			for id, entry := range modelLimiters {
				if now.Sub(entry.lastUsed) > time.Hour {
					delete(modelLimiters, id)
				}
			}
			modelLimitersMu.Unlock()
		}
	}()
}

package modelstate

import (
	"strings"
	"sync"
	"time"

	"awesomeProject/pkg/utils"
)

var (
	temporarilyDisabledModelMu sync.RWMutex
	temporarilyDisabledModel   = make(map[string]time.Time) // model_id -> disabled_until
)

const (
	temporaryModelDisableTTL = 15 * time.Minute
)

func init() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			CleanupTemporarilyDisabledModels()
		}
	}()
}

// DisableModelTemporarily 临时禁用指定模型
func DisableModelTemporarily(modelID string, ttl time.Duration) {
	id := strings.TrimSpace(modelID)
	if id == "" || ttl <= 0 {
		return
	}
	disabledUntil := time.Now().Add(ttl)
	temporarilyDisabledModelMu.Lock()
	temporarilyDisabledModel[id] = disabledUntil
	temporarilyDisabledModelMu.Unlock()
	utils.Logger.Printf("[ClaudeRouter] model_disable: model=%s disabled_until=%s ttl=%s", id, disabledUntil.Format(time.RFC3339), ttl)
}

// IsModelTemporarilyDisabled 检查模型是否被临时禁用
func IsModelTemporarilyDisabled(modelID string) bool {
	id := strings.TrimSpace(modelID)
	if id == "" {
		return false
	}
	now := time.Now()
	temporarilyDisabledModelMu.RLock()
	disabledUntil, ok := temporarilyDisabledModel[id]
	temporarilyDisabledModelMu.RUnlock()
	if !ok {
		return false
	}
	if now.Before(disabledUntil) {
		return true
	}
	temporarilyDisabledModelMu.Lock()
	if currentUntil, currentOK := temporarilyDisabledModel[id]; currentOK && !now.Before(currentUntil) {
		delete(temporarilyDisabledModel, id)
	}
	temporarilyDisabledModelMu.Unlock()
	return false
}

// CleanupTemporarilyDisabledModels 清理过期的临时禁用模型
func CleanupTemporarilyDisabledModels() {
	now := time.Now()
	temporarilyDisabledModelMu.Lock()
	for k, until := range temporarilyDisabledModel {
		if !now.Before(until) {
			delete(temporarilyDisabledModel, k)
		}
	}
	temporarilyDisabledModelMu.Unlock()
}

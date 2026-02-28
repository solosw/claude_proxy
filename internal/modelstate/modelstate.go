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

	conversationModelMu sync.RWMutex
	conversationModel   = make(map[string]ConversationModelEntry) // metadata.user_id -> (model_id, last_seen)
)

type ConversationModelEntry struct {
	ModelID  string
	LastSeen time.Time
}

const (
	TemporaryModelDisableTTL = 1 * time.Minute
	ConversationModelTTL     = 2 *
		TemporaryModelDisableTTL
	ConversationModelCleanupInterval = ConversationModelTTL / 2
)

func init() {
	// 清理临时禁用的模型
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			CleanupTemporarilyDisabledModels()
		}
	}()

	// 清理过期的对话级模型缓存
	go func() {
		ticker := time.NewTicker(ConversationModelCleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			CleanupConversationModels()
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

// ClearAllTemporarilyDisabledModels 清除所有临时禁用的模型（当没有可用模型时调用）
func ClearAllTemporarilyDisabledModels() {
	temporarilyDisabledModelMu.Lock()
	count := len(temporarilyDisabledModel)
	temporarilyDisabledModel = make(map[string]time.Time)
	temporarilyDisabledModelMu.Unlock()
	if count > 0 {
		utils.Logger.Printf("[ClaudeRouter] model_disable_cleared: cleared_count=%d reason=no_available_models", count)
	}
}

// GetConversationModel 获取对话缓存的模型
func GetConversationModel(conversationID string) (string, bool) {
	id := strings.TrimSpace(conversationID)
	if id == "" {
		return "", false
	}
	now := time.Now()
	conversationModelMu.RLock()
	ent := conversationModel[id]
	conversationModelMu.RUnlock()

	if ent.ModelID == "" {
		return "", false
	}
	// TTL 过期视为未缓存
	if ent.LastSeen.IsZero() || now.Sub(ent.LastSeen) > ConversationModelTTL {
		conversationModelMu.Lock()
		delete(conversationModel, id)
		conversationModelMu.Unlock()
		return "", false
	}
	// 更新 last_seen
	conversationModelMu.Lock()
	ent.LastSeen = now
	conversationModel[id] = ent
	conversationModelMu.Unlock()
	return ent.ModelID, true
}

// SetConversationModel 设置对话缓存的模型
func SetConversationModel(conversationID, modelID string) {
	cid := strings.TrimSpace(conversationID)
	mid := strings.TrimSpace(modelID)
	if cid == "" || mid == "" {
		return
	}
	conversationModelMu.Lock()
	conversationModel[cid] = ConversationModelEntry{ModelID: mid, LastSeen: time.Now()}
	conversationModelMu.Unlock()
	utils.Logger.Printf("[ClaudeRouter] conversation_model_set: conversation_id=%s model=%s", cid, mid)
}

// ClearConversationModel 清除对话缓存（出错时调用）
func ClearConversationModel(conversationID string) {
	id := strings.TrimSpace(conversationID)
	if id == "" {
		return
	}
	conversationModelMu.Lock()
	delete(conversationModel, id)
	conversationModelMu.Unlock()
	utils.Logger.Printf("[ClaudeRouter] conversation_model_cleared: conversation_id=%s", id)
}

// CleanupConversationModels 清理过期的对话级模型缓存
func CleanupConversationModels() {
	now := time.Now()
	cutoff := now.Add(-ConversationModelTTL)
	conversationModelMu.Lock()
	for k, v := range conversationModel {
		if v.ModelID == "" || v.LastSeen.Before(cutoff) {
			delete(conversationModel, k)
		}
	}
	conversationModelMu.Unlock()
}

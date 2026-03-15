package task

import (
	"awesomeProject/internal/config"
	"awesomeProject/internal/model"
	"awesomeProject/internal/storage"
	"awesomeProject/pkg/utils"
	"time"
)

const (
	defaultUsageLogCleanupInterval = 24 * time.Hour
	defaultUsageLogRetention       = 24 * time.Hour

	defaultErrorLogCleanupInterval = 30 * time.Minute
	defaultErrorLogRetention       = 12 * time.Hour
)

// CleanupOldUsageLogs 删除过期的使用日志。
func CleanupOldUsageLogs(retention time.Duration) error {
	cutoff := time.Now().Add(-retention)
	result := storage.DB.Where("created_at < ?", cutoff).Delete(&model.UsageLog{})
	if result.Error != nil {
		utils.Logger.Printf("[CleanupTask] delete old usage logs failed: %v", result.Error)
		return result.Error
	}
	utils.Logger.Printf("[CleanupTask] successfully deleted %d old usage logs (before %s)", result.RowsAffected, cutoff.Format(time.RFC3339))
	return nil
}

// CleanupOldErrorLogs 删除过期的错误日志。
func CleanupOldErrorLogs(retention time.Duration) error {
	cutoff := time.Now().Add(-retention)
	result := storage.DB.Where("created_at < ?", cutoff).Delete(&model.ErrorLog{})
	if result.Error != nil {
		utils.Logger.Printf("[CleanupTask] delete old error logs failed: %v", result.Error)
		return result.Error
	}
	utils.Logger.Printf("[CleanupTask] successfully deleted %d old error logs (before %s)", result.RowsAffected, cutoff.Format(time.RFC3339))
	return nil
}

func parseDurationOrDefault(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		utils.Logger.Printf("[Task] invalid duration %q, fallback to %s: %v", s, def, err)
		return def
	}
	if d <= 0 {
		return def
	}
	return d
}

func boolOrDefault(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// StartTasks 启动后台定时任务。
func StartTasks(cfg *config.Config) {
	startUsageLogCleanup(cfg)
	startErrorLogCleanup(cfg)
	startComboWeightAdjust(cfg)
}

func startUsageLogCleanup(cfg *config.Config) {
	interval := defaultUsageLogCleanupInterval
	retention := defaultUsageLogRetention

	// 启动时先执行一次
	if err := CleanupOldUsageLogs(retention); err != nil {
		utils.Logger.Printf("[CleanupTask] initial usage log cleanup failed: %v", err)
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			if err := CleanupOldUsageLogs(retention); err != nil {
				utils.Logger.Printf("[CleanupTask] scheduled usage log cleanup failed: %v", err)
			}
		}
	}()
	utils.Logger.Printf("[CleanupTask] usage log cleanup task started (runs every %s)", interval)
}

func startErrorLogCleanup(cfg *config.Config) {
	// 默认启用；配置为 false 才关闭。
	enabled := true
	interval := defaultErrorLogCleanupInterval
	retention := defaultErrorLogRetention

	if cfg != nil {
		enabled = boolOrDefault(cfg.Tasks.ErrorLogCleanup.Enabled, true)
		interval = parseDurationOrDefault(cfg.Tasks.ErrorLogCleanup.Interval, defaultErrorLogCleanupInterval)
		retention = parseDurationOrDefault(cfg.Tasks.ErrorLogCleanup.Retention, defaultErrorLogRetention)
	}
	if !enabled {
		utils.Logger.Printf("[CleanupTask] error log cleanup task disabled")
		return
	}

	if err := CleanupOldErrorLogs(retention); err != nil {
		utils.Logger.Printf("[CleanupTask] initial error log cleanup failed: %v", err)
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			if err := CleanupOldErrorLogs(retention); err != nil {
				utils.Logger.Printf("[CleanupTask] scheduled error log cleanup failed: %v", err)
			}
		}
	}()
	utils.Logger.Printf("[CleanupTask] error log cleanup task started (runs every %s, retention=%s)", interval, retention)
}

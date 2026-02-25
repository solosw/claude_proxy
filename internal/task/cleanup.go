package task

import (
	"awesomeProject/internal/model"
	"log"
	"time"

	"awesomeProject/internal/storage"
	"awesomeProject/pkg/utils"
)

// CleanupOldLogs 删除7天前的使用日志
func CleanupOldLogs() error {
	// 计算7天前的时间
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)

	result := storage.DB.Where("created_at < ?", sevenDaysAgo).Delete(&model.UsageLog{})
	if result.Error != nil {
		utils.Logger.Printf("[CleanupTask] delete old logs failed: %v", result.Error)
		return result.Error
	}

	utils.Logger.Printf("[CleanupTask] successfully deleted %d old logs (before %s)", result.RowsAffected, sevenDaysAgo.Format("2006-01-02"))
	return nil
}

// StartCleanupTask 启动定时清理任务
// 项目启动时执行一次，然后每天执行一次
func StartCleanupTask() {
	// 项目启动时立即执行一次
	if err := CleanupOldLogs(); err != nil {
		log.Printf("[CleanupTask] initial cleanup failed: %v", err)
	}

	// 启动定时任务，每天执行一次
	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			if err := CleanupOldLogs(); err != nil {
				log.Printf("[CleanupTask] scheduled cleanup failed: %v", err)
			}
		}
	}()

	utils.Logger.Printf("[CleanupTask] cleanup task started (runs daily)")
}

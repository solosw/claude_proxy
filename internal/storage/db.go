package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// DB 是全局数据库句柄，初始化后供其他包使用。
var DB *gorm.DB

// Init 打开 SQLite 数据库，返回 *gorm.DB。
// 表结构的 AutoMigrate 由调用方负责，以避免 import 循环。
func Init(dsn string) (*gorm.DB, error) {
	if dsn == "" {
		dsn = "./data/claude_router.db"
	}

	if err := ensureSQLiteDir(dsn); err != nil {
		return nil, fmt.Errorf("prepare sqlite dsn=%q: %w", dsn, err)
	}

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open sqlite dsn=%q: %w", dsn, err)
	}

	// 开启外键约束（用于 combo -> combo_items 的级联删除等）
	_ = db.Exec("PRAGMA foreign_keys = ON").Error

	DB = db
	return db, nil
}

func ensureSQLiteDir(dsn string) error {
	// 内存数据库不需要目录
	if dsn == ":memory:" || strings.Contains(dsn, "mode=memory") {
		return nil
	}

	path := dsn
	if strings.HasPrefix(path, "file:") {
		path = strings.TrimPrefix(path, "file:")
		// 截断 query 参数（如 file:xxx?cache=shared）
		if i := strings.IndexAny(path, "?;"); i >= 0 {
			path = path[:i]
		}
	}

	path = strings.TrimSpace(path)
	if path == "" || path == ":memory:" {
		return nil
	}

	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}


package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"awesomeProject/internal/storage"
)

func setupUserStoreTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	storage.DB = db
}

func TestAddUserUsage_Accumulates(t *testing.T) {
	setupUserStoreTestDB(t)

	u := &User{
		Username: "usage-u1",
		APIKey:   "usage-key-1",
		Quota:    -1,
	}
	if err := CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := AddUserUsage("usage-u1", 100, 50, 0.01, 0.02); err != nil {
		t.Fatalf("add usage #1: %v", err)
	}
	if err := AddUserUsage("usage-u1", 10, 5, 0.01, 0.02); err != nil {
		t.Fatalf("add usage #2: %v", err)
	}

	got, err := GetUser("usage-u1")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.InputTokens != 110 {
		t.Fatalf("input tokens expect 110, got %d", got.InputTokens)
	}
	if got.OutputTokens != 55 {
		t.Fatalf("output tokens expect 55, got %d", got.OutputTokens)
	}
	if got.TotalTokens != 165 {
		t.Fatalf("total tokens expect 165, got %d", got.TotalTokens)
	}
}

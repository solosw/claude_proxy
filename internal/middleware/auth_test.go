package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"awesomeProject/internal/model"
	"awesomeProject/internal/storage"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	storage.DB = db
}

func TestAPIKeyAuth_QuotaUnlimitedAllows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDB(t)

	u := &model.User{
		Username:    "u1",
		APIKey:      "user-key-1",
		Quota:       -1,
		TotalTokens: 999999,
	}
	if err := model.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	r := gin.New()
	r.Use(APIKeyAuth("admin-key"))
	r.GET("/v1/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	req.Header.Set("X-API-Key", "user-key-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestAPIKeyAuth_ExpiredRejects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDB(t)

	expired := time.Now().Add(-time.Hour)
	u := &model.User{
		Username: "u2",
		APIKey:   "user-key-2",
		Quota:    -1,
		ExpireAt: &expired,
	}
	if err := model.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	r := gin.New()
	r.Use(APIKeyAuth("admin-key"))
	r.GET("/v1/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	req.Header.Set("X-API-Key", "user-key-2")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expect 403, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestRequireAdmin_RejectsNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDB(t)

	u := &model.User{
		Username: "u3",
		APIKey:   "user-key-3",
		Quota:    -1,
		IsAdmin:  false,
	}
	if err := model.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	r := gin.New()
	r.Use(APIKeyAuth("admin-key"))
	r.GET("/admin", RequireAdmin(), func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("X-API-Key", "user-key-3")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expect 403, got %d, body=%s", w.Code, w.Body.String())
	}
}

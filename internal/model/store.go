package model

import (
	"errors"
	"strings"
	"sync"
	"time"

	"awesomeProject/internal/storage"
	"gorm.io/gorm"
)

var (
	// ErrNotFound 在指定 ID 不存在时返回。
	ErrNotFound = errors.New("not found")

	comboIDCache   map[string]struct{}
	comboIDCacheMu sync.RWMutex
)

// ListModels 返回所有模型的切片副本。
func ListModels() []*Model {
	var ms []*Model
	if err := storage.DB.Find(&ms).Error; err != nil {
		return []*Model{}
	}
	return ms
}

func GetModel(id string) (*Model, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrNotFound
	}
	var m Model
	if err := storage.DB.Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

func CreateModel(m *Model) error {
	if m == nil || m.ID == "" {
		return errors.New("invalid model")
	}
	if err := storage.DB.Create(m).Error; err != nil {
		return err
	}
	return nil
}

func UpdateModel(id string, m *Model) error {
	if m == nil {
		return errors.New("invalid model")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrNotFound
	}
	m.ID = id
	if err := storage.DB.Save(m).Error; err != nil {
		return err
	}
	return nil
}

func DeleteModel(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrNotFound
	}
	return storage.DB.Where("id = ?", id).Delete(&Model{}).Error
}

func ListUsers() ([]*User, error) {
	var users []*User
	if err := storage.DB.Order("created_at desc").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func GetUser(username string) (*User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, ErrNotFound
	}
	var u User
	if err := storage.DB.Where("username = ?", username).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func GetUserByAPIKey(apiKey string) (*User, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, ErrNotFound
	}
	var u User
	if err := storage.DB.Where("api_key = ?", apiKey).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func CreateUser(u *User) error {
	if u == nil {
		return errors.New("invalid user")
	}
	u.Username = strings.TrimSpace(u.Username)
	u.APIKey = strings.TrimSpace(u.APIKey)
	if u.Username == "" || u.APIKey == "" {
		return errors.New("username and api_key required")
	}
	if u.Quota < -1 {
		return errors.New("quota must be -1 or >= 0")
	}
	return storage.DB.Create(u).Error
}

func UpdateUserByUsername(username string, update map[string]any) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return ErrNotFound
	}
	if len(update) == 0 {
		return nil
	}
	if v, ok := update["api_key"]; ok {
		if s, _ := v.(string); strings.TrimSpace(s) == "" {
			return errors.New("api_key required")
		}
	}
	if v, ok := update["quota"]; ok {
		switch q := v.(type) {
		case int64:
			if q < -1 {
				return errors.New("quota must be -1 or >= 0")
			}
		case int:
			if q < -1 {
				return errors.New("quota must be -1 or >= 0")
			}
		}
	}
	res := storage.DB.Model(&User{}).Where("username = ?", username).Updates(update)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func DeleteUser(username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return ErrNotFound
	}
	res := storage.DB.Where("username = ?", username).Delete(&User{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func AddUserUsage(username string, inputTokens, outputTokens int64, inputPrice, outputPrice float64) error {
	if strings.TrimSpace(username) == "" {
		return nil
	}
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	if inputPrice < 0 {
		inputPrice = 0
	}
	if outputPrice < 0 {
		outputPrice = 0
	}
	total := inputTokens + outputTokens
	// 计算总费用：(inputTokens / 1000) * inputPrice + (outputTokens / 1000) * outputPrice
	totalCost := (float64(inputTokens)/1000)*inputPrice + (float64(outputTokens)/1000)*outputPrice

	return storage.DB.Model(&User{}).Where("username = ?", username).Updates(map[string]any{
		"input_tokens":  gorm.Expr("input_tokens + ?", inputTokens),
		"output_tokens": gorm.Expr("output_tokens + ?", outputTokens),
		"total_tokens":  gorm.Expr("total_tokens + ?", total),
		"quota":         gorm.Expr(`
    CASE
        WHEN quota < 0 THEN -1
        WHEN quota - ? < 0 THEN 0
        ELSE quota - ?
    END
`, totalCost, totalCost),
		"updated_at":    time.Now(),
	}).Error
}

// RecordUsageLog 记录单次请求的 token 使用日志。
func RecordUsageLog(username string, m Model, inputTokens, outputTokens int64, inputPrice, outputPrice float64) error {
	if strings.TrimSpace(username) == "" || strings.TrimSpace(m.ID) == "" {
		return nil
	}
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	if inputPrice < 0 {
		inputPrice = 0
	}
	if outputPrice < 0 {
		outputPrice = 0
	}

	// 计算总费用：(inputTokens / 1000) * inputPrice + (outputTokens / 1000) * outputPrice
	totalCost := (float64(inputTokens)/1000)*inputPrice + (float64(outputTokens)/1000)*outputPrice

	log := &UsageLog{
		Username:     username,
		ModelID:      m.ID,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		InputPrice:   inputPrice,
		OutputPrice:  outputPrice,
		TotalCost:    totalCost,
		Provider:     m.Provider,
		CreatedAt:    time.Now(),
	}
	return storage.DB.Create(log).Error
}

// GetUsageLogsByUsername 获取指定用户的使用日志（分页）
func GetUsageLogsByUsername(username string, page, pageSize int) ([]UsageLog, int64, error) {
	if strings.TrimSpace(username) == "" || page < 1 || pageSize < 1 {
		return nil, 0, nil
	}

	var logs []UsageLog
	var total int64

	// 查询总数
	if err := storage.DB.Where("username = ?", username).Model(&UsageLog{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 查询分页数据
	offset := (page - 1) * pageSize
	if err := storage.DB.Where("username = ?", username).
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// ListCombos 返回所有组合模型。
func ListCombos() []*Combo {
	var cs []*Combo
	if err := storage.DB.Preload("Items").Find(&cs).Error; err != nil {
		return []*Combo{}
	}
	return cs
}

// IsComboID 返回 id 是否为已存在的 combo id（用于先判再查，避免对纯 model id 误调 GetCombo 产生 record not found）。
func IsComboID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	comboIDCacheMu.RLock()
	cache := comboIDCache
	comboIDCacheMu.RUnlock()
	if cache == nil {
		comboIDCacheMu.Lock()
		if comboIDCache == nil {
			var ids []string
			_ = storage.DB.Model(&Combo{}).Pluck("id", &ids).Error
			comboIDCache = make(map[string]struct{}, len(ids))
			for _, s := range ids {
				comboIDCache[s] = struct{}{}
			}
		}
		cache = comboIDCache
		comboIDCacheMu.Unlock()
	}
	comboIDCacheMu.RLock()
	_, ok := cache[id]
	comboIDCacheMu.RUnlock()
	return ok
}

func invalidateComboIDCache() {
	comboIDCacheMu.Lock()
	comboIDCache = nil
	comboIDCacheMu.Unlock()
}

func GetCombo(id string) (*Combo, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrNotFound
	}
	var c Combo
	// 使用 Where 明确条件，避免 GORM 日志/某些驱动把字符串用双引号内联导致 SQLite 将值误当作列名
	if err := storage.DB.Where("id = ?", id).Preload("Items").First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func CreateCombo(c *Combo) error {
	if c == nil || c.ID == "" {
		return errors.New("invalid combo")
	}
	for i := range c.Items {
		c.Items[i].ComboID = c.ID
	}
	err := storage.DB.Transaction(func(tx *gorm.DB) error {
		return tx.Create(c).Error
	})
	if err == nil {
		invalidateComboIDCache()
	}
	return err
}

func UpdateCombo(id string, c *Combo) error {
	if c == nil {
		return errors.New("invalid combo")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrNotFound
	}
	c.ID = id
	for i := range c.Items {
		c.Items[i].ComboID = id
	}

	err := storage.DB.Transaction(func(tx *gorm.DB) error {
		// 先更新 combo 主表
		if err := tx.Model(&Combo{}).Where("id = ?", id).Updates(map[string]any{
			"name":        c.Name,
			"description": c.Description,
			"enabled":     c.Enabled,
		}).Error; err != nil {
			return err
		}

		// 替换 items（简单实现：先删后插）
		if err := tx.Where("combo_id = ?", id).Delete(&ComboItem{}).Error; err != nil {
			return err
		}
		if len(c.Items) > 0 {
			if err := tx.Create(&c.Items).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		invalidateComboIDCache()
	}
	return err
}

func DeleteCombo(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrNotFound
	}
	return storage.DB.Transaction(func(tx *gorm.DB) error {
		// 保险起见显式删子表（即便外键未开启也能工作）
		if err := tx.Where("combo_id = ?", id).Delete(&ComboItem{}).Error; err != nil {
			return err
		}
		err := tx.Where("id = ?", id).Delete(&Combo{}).Error
		if err == nil {
			invalidateComboIDCache()
		}
		return err
	})
}

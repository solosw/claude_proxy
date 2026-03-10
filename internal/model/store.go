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

// ListModelsWithPage 分页查询模型列表，支持按名称模糊搜索（不区分大小写）
func ListModelsWithPage(name string, page, pageSize int) ([]*Model, int64, error) {
	var models []*Model
	var total int64

	// 构建查询
	query := storage.DB.Model(&Model{})

	// 按名称模糊搜索（不区分大小写）
	if name != "" {
		query = query.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(name)+"%")
	}

	// 查询总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Order("id ASC").Limit(pageSize).Offset(offset).Find(&models).Error; err != nil {
		return nil, 0, err
	}

	return models, total, nil
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

// ListUsersWithPage 分页查询用户列表
func ListUsersWithPage(username, apiKey string, isAdmin *bool, page, pageSize int) ([]*User, int64, error) {
	var users []*User
	var total int64

	// 构建查询
	query := storage.DB.Model(&User{})

	// 按用户名筛选
	if username != "" {
		query = query.Where("username LIKE ?", "%"+username+"%")
	}

	// 按 API Key 筛选
	if apiKey != "" {
		query = query.Where("api_key LIKE ?", "%"+apiKey+"%")
	}

	// 按管理员状态筛选
	if isAdmin != nil {
		query = query.Where("is_admin = ?", *isAdmin)
	}

	// 查询总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Order("created_at desc").Limit(pageSize).Offset(offset).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
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

	// 先获取用户的计费模式
	var user User
	err := storage.DB.Where("username = ?", username).First(&user).Error
	if err != nil {
		return err
	}

	var totalCost float64
	billingMode := user.BillingMode
	if billingMode == "" {
		billingMode = "token" // 默认按 token 计费
	}

	if billingMode == "request" {
		// 按次数计费：费用 = 单次价格 * 1
		totalCost = user.RequestPrice
	} else {
		// 按 token 计费
		totalCost = (float64(inputTokens)/1000000)*inputPrice + (float64(outputTokens)/1000000)*outputPrice
	}

	updates := map[string]any{
		"input_tokens":    gorm.Expr("input_tokens + ?", inputTokens),
		"output_tokens":   gorm.Expr("output_tokens + ?", outputTokens),
		"total_tokens":    gorm.Expr("total_tokens + ?", total),
		"total_requests":  gorm.Expr("total_requests + 1"), // 每次请求都累计
		"quota": gorm.Expr(`
    CASE
        WHEN quota < 0 THEN -1
        WHEN quota - ? < 0 THEN 0
        ELSE quota - ?
    END
`, totalCost, totalCost),
		"updated_at": time.Now(),
	}

	return storage.DB.Model(&User{}).Where("username = ?", username).Updates(updates).Error
}

// RecordUsageLog 记录单次请求的 token 使用日志。
func RecordUsageLog(username string, m Model, inputTokens, outputTokens int64, inputPrice, outputPrice float64, combo Combo) error {
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

	// 先获取用户的计费模式
	var user User
	err := storage.DB.Where("username = ?", username).First(&user).Error
	if err != nil {
		return err
	}

	billingMode := user.BillingMode
	if billingMode == "" {
		billingMode = "token" // 默认按 token 计费
	}

	var totalCost float64
	var requestCount int64
	var requestPrice float64

	if billingMode == "request" {
		// 按次数计费
		requestCount = 1
		requestPrice = user.RequestPrice
		totalCost = requestPrice
	} else {
		// 按 token 计费
		totalCost = (float64(inputTokens)/1000000)*inputPrice + (float64(outputTokens)/1000000)*outputPrice
	}

	log := &UsageLog{
		Username:      username,
		ModelID:       combo.ID,
		InputTokens:   inputTokens,
		OutputTokens:  outputTokens,
		InputPrice:    inputPrice,
		OutputPrice:   outputPrice,
		TotalCost:     totalCost,
		Provider:      combo.Provider,
		BillingMode:   billingMode,
		RequestCount:  requestCount,
		RequestPrice:  requestPrice,
		CreatedAt:     time.Now(),
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
func GetComboIgnoreError(id string) *Combo {
	id = strings.TrimSpace(id)
	if id == "" {
		return &Combo{
			Name:     "auto",
			ID:       "auto",
			Provider: "kiro",
		}
	}
	var c Combo
	// 使用 Where 明确条件，避免 GORM 日志/某些驱动把字符串用双引号内联导致 SQLite 将值误当作列名
	if err := storage.DB.Where("id = ?", id).Preload("Items").First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &Combo{
				Name:     "auto",
				ID:       "auto",
				Provider: "kiro",
			}
		}
		return &Combo{
			Name:     "auto",
			ID:       "auto",
			Provider: "kiro",
		}
	}
	return &c
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
			"name":         c.Name,
			"description":  c.Description,
			"enabled":      c.Enabled,
			"provider":     c.Provider,
			"input_price":  c.InputPrice,
			"output_price": c.OutputPrice,
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

// RecordErrorLog 记录一条模型调用失败日志。
func RecordErrorLog(modelID, username string, statusCode int, errMsg string) error {
	if strings.TrimSpace(modelID) == "" {
		return nil
	}
	msg := errMsg
	if len(msg) > 2048 {
		msg = msg[:2000]
	}
	entry := &ErrorLog{
		ModelID:    modelID,
		Username:   username,
		StatusCode: statusCode,
		ErrorMsg:   msg,
		CreatedAt:  time.Now(),
	}
	return storage.DB.Create(entry).Error
}

// ListErrorLogs 查询错误日志，支持按 modelID 筛选和分页。
func ListErrorLogs(modelID string, page, pageSize int) ([]ErrorLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := storage.DB.Model(&ErrorLog{})
	if strings.TrimSpace(modelID) != "" {
		query = query.Where("model_id = ?", strings.TrimSpace(modelID))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var logs []ErrorLog
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// ==================== 兑换码相关操作 ====================

var (
	ErrRedeemCodeNotFound    = errors.New("redeem code not found")
	ErrRedeemCodeExpired     = errors.New("redeem code expired")
	ErrRedeemCodeExhausted   = errors.New("redeem code exhausted")
	ErrRedeemCodeAlreadyUsed = errors.New("you have already used this redeem code")
)

// CreateRedeemCode 创建兑换码
func CreateRedeemCode(code *RedeemCode) error {
	if code == nil {
		return errors.New("invalid redeem code")
	}
	code.Code = strings.TrimSpace(code.Code)
	if code.Code == "" {
		return errors.New("code required")
	}
	if code.Quota <= 0 {
		return errors.New("quota must be > 0")
	}
	if code.MaxUses < 1 {
		code.MaxUses = 1
	}
	code.UsedCount = 0
	code.CreatedAt = time.Now()
	code.UpdatedAt = time.Now()
	return storage.DB.Create(code).Error
}

// GetRedeemCode 根据兑换码获取详情
func GetRedeemCode(code string) (*RedeemCode, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, ErrRedeemCodeNotFound
	}
	var rc RedeemCode
	if err := storage.DB.Where("code = ?", code).First(&rc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRedeemCodeNotFound
		}
		return nil, err
	}
	return &rc, nil
}

// ListRedeemCodesWithPage 分页查询兑换码列表
func ListRedeemCodesWithPage(code, createdBy string, page, pageSize int) ([]*RedeemCode, int64, error) {
	var codes []*RedeemCode
	var total int64

	query := storage.DB.Model(&RedeemCode{})

	if code != "" {
		query = query.Where("code LIKE ?", "%"+code+"%")
	}
	if createdBy != "" {
		query = query.Where("created_by = ?", createdBy)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&codes).Error; err != nil {
		return nil, 0, err
	}

	return codes, total, nil
}

// DeleteRedeemCode 删除兑换码
func DeleteRedeemCode(id int64) error {
	if id <= 0 {
		return errors.New("invalid id")
	}
	res := storage.DB.Where("id = ?", id).Delete(&RedeemCode{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrRedeemCodeNotFound
	}
	return nil
}

// RedeemQuota 用户兑换额度（原子操作，防止并发问题）
func RedeemQuota(code, username string) error {
	code = strings.TrimSpace(code)
	username = strings.TrimSpace(username)
	if code == "" || username == "" {
		return errors.New("code and username required")
	}

	// 使用事务确保原子性
	return storage.DB.Transaction(func(tx *gorm.DB) error {
		// 1. 查询兑换码（加锁）
		var rc RedeemCode
		if err := tx.Where("code = ?", code).First(&rc).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRedeemCodeNotFound
			}
			return err
		}

		// 2. 检查是否过期
		if rc.ExpireAt != nil && time.Now().After(*rc.ExpireAt) {
			return ErrRedeemCodeExpired
		}

		// 3. 检查是否已用完
		if rc.UsedCount >= rc.MaxUses {
			return ErrRedeemCodeExhausted
		}

		// 4. 检查用户是否已使用过该兑换码
		var count int64
		if err := tx.Model(&RedeemLog{}).Where("code = ? AND username = ?", code, username).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrRedeemCodeAlreadyUsed
		}

		// 5. 增加用户额度
		if err := tx.Model(&User{}).Where("username = ?", username).Update("quota", gorm.Expr("quota + ?", rc.Quota)).Error; err != nil {
			return err
		}

		// 6. 更新兑换码使用次数
		if err := tx.Model(&RedeemCode{}).Where("id = ?", rc.ID).Updates(map[string]any{
			"used_count": gorm.Expr("used_count + 1"),
			"updated_at": time.Now(),
		}).Error; err != nil {
			return err
		}

		// 7. 记录兑换日志
		log := &RedeemLog{
			Code:      code,
			Username:  username,
			Quota:     rc.Quota,
			CreatedAt: time.Now(),
		}
		if err := tx.Create(log).Error; err != nil {
			return err
		}

		return nil
	})
}

// ListRedeemLogsByUsername 查询用户的兑换记录
func ListRedeemLogsByUsername(username string, page, pageSize int) ([]RedeemLog, int64, error) {
	if strings.TrimSpace(username) == "" || page < 1 || pageSize < 1 {
		return nil, 0, nil
	}

	var logs []RedeemLog
	var total int64

	if err := storage.DB.Where("username = ?", username).Model(&RedeemLog{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

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

package model

import (
	"errors"
	"strings"
	"sync"

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

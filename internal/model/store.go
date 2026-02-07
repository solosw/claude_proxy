package model

import (
	"errors"

	"awesomeProject/internal/storage"
	"gorm.io/gorm"
)

var (
	// ErrNotFound 在指定 ID 不存在时返回。
	ErrNotFound = errors.New("not found")
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
	var m Model
	if err := storage.DB.First(&m, "id = ?", id).Error; err != nil {
		if errors.Is(storage.DB.First(&m).Error, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
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
	m.ID = id
	if err := storage.DB.Save(m).Error; err != nil {
		return err
	}
	return nil
}

func DeleteModel(id string) error {
	if err := storage.DB.Delete(&Model{}, "id = ?", id).Error; err != nil {
		return err
	}
	return nil
}

// ListCombos 返回所有组合模型。
func ListCombos() []*Combo {
	var cs []*Combo
	if err := storage.DB.Preload("Items").Find(&cs).Error; err != nil {
		return []*Combo{}
	}
	return cs
}

func GetCombo(id string) (*Combo, error) {
	var c Combo
	if err := storage.DB.Preload("Items").First(&c, "id = ?", id).Error; err != nil {
		return nil, ErrNotFound
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
	return storage.DB.Transaction(func(tx *gorm.DB) error {
		return tx.Create(c).Error
	})
}

func UpdateCombo(id string, c *Combo) error {
	if c == nil {
		return errors.New("invalid combo")
	}
	c.ID = id
	for i := range c.Items {
		c.Items[i].ComboID = id
	}

	return storage.DB.Transaction(func(tx *gorm.DB) error {
		// 先更新 combo 主表
		if err := tx.Model(&Combo{}).Where("id = ?", id).Updates(map[string]any{
			"name":        c.Name,
			"description": c.Description,
			"enabled":     c.Enabled,
		}).Error; err != nil {
			return err
		}

		// 替换 items（简单实现：先删后插）
		if err := tx.Delete(&ComboItem{}, "combo_id = ?", id).Error; err != nil {
			return err
		}
		if len(c.Items) > 0 {
			if err := tx.Create(&c.Items).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func DeleteCombo(id string) error {
	return storage.DB.Transaction(func(tx *gorm.DB) error {
		// 保险起见显式删子表（即便外键未开启也能工作）
		if err := tx.Delete(&ComboItem{}, "combo_id = ?", id).Error; err != nil {
			return err
		}
		return tx.Delete(&Combo{}, "id = ?", id).Error
	})
}

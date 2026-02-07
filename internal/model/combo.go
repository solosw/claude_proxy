package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// StringSlice 用于将 []string 以 JSON 形式存入数据库 TEXT 字段。
type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	b, err := json.Marshal([]string(s))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (s *StringSlice) Scan(value any) error {
	if value == nil {
		*s = nil
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("unsupported Scan type: %T", value)
	}

	if len(data) == 0 {
		*s = nil
		return nil
	}

	var out []string
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	*s = out
	return nil
}

// ComboItem 表示组合模型中的一个子模型及其权重、关键词等。
type ComboItem struct {
	ID      uint   `json:"-" gorm:"primaryKey"`
	ComboID string `json:"-" gorm:"index"` // 外键，指向 Combo.ID

	ModelID  string      `json:"model_id"`
	Weight   float64     `json:"weight"`
	Keywords StringSlice `json:"keywords,omitempty" gorm:"type:text"`
}

// Combo 表示一个组合模型（虚拟模型），对外表现为一个普通模型 ID。
type Combo struct {
	ID          string `json:"id" gorm:"primaryKey"` // 唯一 ID，例如 "combo:fast-and-cheap"
	Name        string `json:"name"`                 // 展示名
	Description string      `json:"description"`
	Items       []ComboItem `json:"items" gorm:"foreignKey:ComboID;references:ID;constraint:OnDelete:CASCADE"`
	Enabled     bool        `json:"enabled"`
}


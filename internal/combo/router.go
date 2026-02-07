package combo

import (
	"math/rand"
	"strings"
	"time"

	"awesomeProject/internal/model"
)

// ChooseModelID 根据 combo 的 items、权重和关键词，从输入文本中选择一个子模型。
//
// 策略（先简单可用，后续可升级）：
// - **关键词命中优先**：若某个 item 的任一 keyword 命中输入文本，则只在命中的 items 中选择权重最高的
// - **否则按权重随机**：按 weight 做加权随机（weight<=0 视为 0）
func ChooseModelID(c *model.Combo, inputText string) string {
	if c == nil || len(c.Items) == 0 {
		return ""
	}

	text := strings.ToLower(inputText)

	// 1) 关键词命中优先
	var (
		bestID     string
		bestWeight float64 = -1
		found      bool
	)
	for _, it := range c.Items {
		if it.ModelID == "" {
			continue
		}
		if len(it.Keywords) == 0 {
			continue
		}
		for _, kw := range it.Keywords {
			kw = strings.ToLower(strings.TrimSpace(kw))
			if kw == "" {
				continue
			}
			if strings.Contains(text, kw) {
				found = true
				if it.Weight > bestWeight {
					bestWeight = it.Weight
					bestID = it.ModelID
				}
				break
			}
		}
	}
	if found && bestID != "" {
		return bestID
	}

	// 2) 按权重随机
	total := 0.0
	for _, it := range c.Items {
		if it.ModelID == "" {
			continue
		}
		if it.Weight > 0 {
			total += it.Weight
		}
	}
	if total <= 0 {
		// 全部权重 <=0，退化为第一个有效 item
		for _, it := range c.Items {
			if it.ModelID != "" {
				return it.ModelID
			}
		}
		return ""
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	x := r.Float64() * total
	for _, it := range c.Items {
		if it.ModelID == "" || it.Weight <= 0 {
			continue
		}
		x -= it.Weight
		if x <= 0 {
			return it.ModelID
		}
	}

	// 兜底
	return c.Items[0].ModelID
}


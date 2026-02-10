package combo

import (
	"awesomeProject/internal/model"
	"log"
	"regexp"
	"sort"
	"strings"
)

func HasCustomBangKeyword(text string, keywords []string) bool {
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" || !strings.HasPrefix(kw, "!") {
			continue // 跳过无效关键词
		}

		base := kw[1:] // 提取 ! 后面的主体部分

		var pattern string
		if base == "" {
			// 关键词是 "!" 本身
			pattern = `!($|[\s\p{P}\p{S}])`
		} else {
			// 关键词是 "!xxx"，只转义 "xxx" 部分
			pattern = `!` + regexp.QuoteMeta(base) + `($|[\s\p{P}\p{S}])`
		}

		matched, err := regexp.MatchString(pattern, text)
		if err != nil {
			// 如果仍然出错，可以打印或继续下一个关键词
			continue
		}
		if matched {
			return true
		}
	}
	return false
}

// ChooseModelID 根据 combo 的 items、权重和关键词，从输入文本中选择一个子模型。
//
// 策略（先简单可用，后续可升级）：
// - **关键词命中优先**：若某个 item 的任一 keyword 命中输入文本，则只在命中的 items 中选择权重最高的
// - **否则按权重随机**：按 weight 做加权随机（weight<=0 视为 0）
func ChooseModelID(c *model.Combo, inputText string) string {
	if c == nil || len(c.Items) == 0 {
		return ""
	}

	text := strings.TrimSpace(inputText)
	log.Printf("Input Text: %s\n", text)

	// 1) 关键词命中优先
	var (
		bestID string

		found bool
	)
	sort.Slice(c.Items, func(i, j int) bool {
		return c.Items[i].Weight > c.Items[j].Weight
	})
	for _, it := range c.Items {
		if it.ModelID == "" {
			continue
		}
		if len(it.Keywords) == 0 {
			continue
		}

		if HasCustomBangKeyword(text, it.Keywords) {

			bestID = it.ModelID
			found = true
			break
		}

	}
	if found && bestID != "" {
		return bestID
	}

	return c.Items[0].ModelID
}

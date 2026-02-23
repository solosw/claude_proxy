package main

import (
	"fmt"
	"regexp"
	"strings"
)

func main() {
	keywords := []string{"!文档", "!审批", "!help"}

	testCases := []string{
		"请查看!文档",      // ✅ true
		"需要!审批流程",     // ❌ false（因为后面是“流程”）
		"运行!审批",       // ✅ true
		"输入!help获取帮助", // ❌ false（“获取”紧跟）
		"命令!help",     // ✅ true（后面有空格）
		"!help      " +
			"123", // ✅ true（后面是标点）
		"!文档2.", // ❌ false
		"没有指令",  // ❌ false
	}

	for _, text := range testCases {
		hit := HasCustomBangKeyword(text, keywords)
		fmt.Printf("%-20s -> %v\n", text, hit)
	}
}

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

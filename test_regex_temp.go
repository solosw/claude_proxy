package main

import (
	"fmt"
	"regexp"
)

func main() {
	patternCN := regexp.MustCompile(`(?i)(?:你 您)(?::|：)?(?:是|叫|属于|基于|使用|用的|叫啥|是啥)?(?:(?:什么|啥|哪种|哪个)?(?:模型|大模型|AI|人工智能|机器人)?|人|谁|什么(?:人|家伙))`)
	patternEN := regexp.MustCompile(`(?i)\b(?:what\s*(?:model|AI|model\s+are\s+you|model\s+is\s+this|model\s+name|are\s+you\s+using)|which\s+model|who\s+are\s+you|what(?:\s+is)?\s+your\s+name|gpt[-\s]?\d+|llama\s*\d*|qwen\s*\d*|mistral\s*\d*)\b`)

	tests := []string{
		"你是什么模型？",
		"你叫什么？",
		"你是谁？",
		"你是啥模型",
		"你是基于什么模型？",
		"您是什么AI？",
		"你是叫什么名字？",
		"你是啥",
		"what model are you?",
		"who are you",
		"what's your name",
		"which model",
		"你好",
		"今天天气怎么样",
		"帮我写一段代码",
	}

	for _, t := range tests {
		matchedCN := patternCN.MatchString(t)
		matchedEN := patternEN.MatchString(t)
		if matchedCN || matchedEN {
			fmt.Printf("✅ 匹配: %q\n", t)
		} else {
			fmt.Printf("❌ 未匹配: %q\n", t)
		}
	}
}

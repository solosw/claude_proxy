package utils

import (
	"log"
	"os"
)

// Logger 是应用使用的基础 logger，输出到标准输出，带时间和短文件名。
var Logger = log.New(os.Stdout, "[ClaudeRouter] ", log.LstdFlags|log.Lshortfile)


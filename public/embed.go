package public

import "embed"

// WebFS 嵌入编译后的 Vue 前端资源（public/web 下的内容）。
//go:embed web/*
var WebFS embed.FS


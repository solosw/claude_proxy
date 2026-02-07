package utils

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ProxySSE 将上游返回的 SSE 流透传给客户端。
// 上层负责控制生命周期（如 context 取消、错误处理等）。
func ProxySSE(c *gin.Context, src io.ReadCloser) {
	defer src.Close()

	w := c.Writer
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")

	c.Status(http.StatusOK)

	ctx := c.Request.Context()
	buf := make([]byte, 4096)
	flusher, ok := w.(http.Flusher)
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := src.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				break
			}
			if ok {
				flusher.Flush()
			}
		}
		if err != nil {
			break
		}
	}
}


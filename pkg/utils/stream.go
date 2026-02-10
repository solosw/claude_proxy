package utils

import (
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// SSE 写超时：若客户端在此时间内未读取（如 Claude Code 执行耗时命令导致无响应），服务端会主动关闭连接。
const sseWriteTimeout = 30 * time.Minute

// ProxySSE 将上游返回的 SSE 流透传给客户端。
// 当客户端长时间不读取时（如执行耗时命令），通过写超时主动关闭 SSE，避免连接一直挂起。
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
	// 用于写超时：客户端长时间不读时 Write 会超时，服务端主动结束
	writeController := http.NewResponseController(w)
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := src.Read(buf)
		if n > 0 {
			if writeController != nil {
				_ = writeController.SetWriteDeadline(time.Now().Add(sseWriteTimeout))
			}
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

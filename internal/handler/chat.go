package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"awesomeProject/internal/model"
	"awesomeProject/internal/provider"
	reqtrans "awesomeProject/internal/translator/request"
	resptrans "awesomeProject/internal/translator/response"
	"awesomeProject/pkg/utils"
)

type ChatHandler struct {
}

func NewChatHandler() *ChatHandler {
	return &ChatHandler{}
}

// RegisterRoutes 注册 /v1/chat/completions 端点（OpenAI 兼容）。
func (h *ChatHandler) RegisterRoutes(r gin.IRoutes) {
	r.POST("/v1/chat/completions", h.handleChatCompletions)
}

func (h *ChatHandler) handleChatCompletions(c *gin.Context) {
	var req reqtrans.OpenAIChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	// 从请求中的 model 字段查找数据库中的模型配置
	modelID := strings.TrimSpace(req.Model)
	if modelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing model"})
		return
	}

	m, err := model.GetModel(modelID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found: " + modelID})
		return
	}
	if !m.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model disabled: " + m.ID})
		return
	}

	interfaceType := strings.TrimSpace(m.Interface)
	if interfaceType == "" {
		interfaceType = "openai"
	}
	if interfaceType != "openai" && interfaceType != "openai_compatible" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "this endpoint only supports interface_type=openai (got " + interfaceType + ")",
		})
		return
	}

	// 从模型配置读取 api_key / base_url
	apiKey := strings.TrimSpace(m.APIKey)
	baseURL := strings.TrimRight(strings.TrimSpace(m.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	upstreamModel := strings.TrimSpace(m.UpstreamID)
	if upstreamModel == "" {
		upstreamModel = m.ID
	}

	// 创建 OpenAI provider 实例
	oai := provider.NewOpenAI(provider.OpenAIConfig{
		Name:           m.ID,
		APIKey:         apiKey,
		BaseURL:        baseURL,
		TimeoutSeconds: 60,
	})

	chatReq := req.ToChatRequest()
	if chatReq == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat request"})
		return
	}
	chatReq.Model = upstreamModel

	if req.Stream {
		body, err := oai.ChatStream(c.Request.Context(), chatReq)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		utils.ProxySSE(c, body)
		return
	}

	resp, err := oai.Chat(c.Request.Context(), chatReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	out := resptrans.ToOpenAIChatCompletionResponse(resp)
	c.JSON(http.StatusOK, out)
}


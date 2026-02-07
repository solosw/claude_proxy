package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	appconfig "awesomeProject/internal/config"
	"awesomeProject/internal/model"
)

// RegisterModelRoutes 注册模型、组合模型、运营商列表接口。运营商为系统内置，仅可读配置列表。
func RegisterModelRoutes(r gin.IRouter, cfg *appconfig.Config) {
	api := r.Group("/api")

	// 模型 CRUD
	api.GET("/models", listModels)
	api.POST("/models", createModel)
	api.GET("/models/:id", getModel)
	api.PUT("/models/:id", updateModel)
	api.DELETE("/models/:id", deleteModel)

	// 运营商：系统内置，仅列表与详情（来自配置，不可增删改）
	api.GET("/operators", listOperators(cfg))
	api.GET("/operators/:id", getOperator(cfg))

	// 组合模型 CRUD
	api.GET("/combos", listCombos)
	api.POST("/combos", createCombo)
	api.GET("/combos/:id", getCombo)
	api.PUT("/combos/:id", updateCombo)
	api.DELETE("/combos/:id", deleteCombo)
}

func listModels(c *gin.Context) {
	ms := model.ListModels()
	c.JSON(http.StatusOK, ms)
}

func getModel(c *gin.Context) {
	id := c.Param("id")
	m, err := model.GetModel(id)
	if err != nil {
		status := http.StatusInternalServerError
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, m)
}

func createModel(c *gin.Context) {
	var m model.Model
	if err := c.ShouldBindJSON(&m); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := model.CreateModel(&m); err != nil {
		status := http.StatusBadRequest
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, m)
}

func updateModel(c *gin.Context) {
	id := c.Param("id")
	var m model.Model
	if err := c.ShouldBindJSON(&m); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := model.UpdateModel(id, &m); err != nil {
		status := http.StatusBadRequest
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, m)
}

func deleteModel(c *gin.Context) {
	id := c.Param("id")
	if err := model.DeleteModel(id); err != nil {
		status := http.StatusInternalServerError
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func listOperators(cfg *appconfig.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cfg == nil || cfg.Operators == nil {
			c.JSON(http.StatusOK, []any{})
			return
		}
		list := make([]*model.Operator, 0, len(cfg.Operators))
		for id, ep := range cfg.Operators {
			list = append(list, &model.Operator{
				ID:          id,
				Name:        strings.TrimSpace(ep.Name),
				Description: strings.TrimSpace(ep.Description),
				Enabled:     ep.Enabled,
			})
		}
		c.JSON(http.StatusOK, list)
	}
}

func getOperator(cfg *appconfig.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if cfg == nil || cfg.Operators == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		ep, ok := cfg.Operators[id]
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "operator not found: " + id})
			return
		}
		c.JSON(http.StatusOK, &model.Operator{
			ID:          id,
			Name:        strings.TrimSpace(ep.Name),
			Description: strings.TrimSpace(ep.Description),
			Enabled:     ep.Enabled,
		})
	}
}

func listCombos(c *gin.Context) {
	cs := model.ListCombos()
	c.JSON(http.StatusOK, cs)
}

func getCombo(c *gin.Context) {
	id := c.Param("id")
	cb, err := model.GetCombo(id)
	if err != nil {
		status := http.StatusInternalServerError
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cb)
}

func createCombo(c *gin.Context) {
	var cb model.Combo
	if err := c.ShouldBindJSON(&cb); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := model.CreateCombo(&cb); err != nil {
		status := http.StatusBadRequest
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, cb)
}

func updateCombo(c *gin.Context) {
	id := c.Param("id")
	var cb model.Combo
	if err := c.ShouldBindJSON(&cb); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := model.UpdateCombo(id, &cb); err != nil {
		status := http.StatusBadRequest
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cb)
}

func deleteCombo(c *gin.Context) {
	id := c.Param("id")
	if err := model.DeleteCombo(id); err != nil {
		status := http.StatusInternalServerError
		if err == model.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}


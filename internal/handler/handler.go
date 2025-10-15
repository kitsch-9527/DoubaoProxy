package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"DoubaoProxy/internal/model"
	"DoubaoProxy/internal/service/doubao"
)

// Register 将业务路由挂载到 gin 引擎上。
func Register(router *gin.Engine, service *doubao.Service) {
	h := &handler{service: service}

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	api := router.Group("/api")
	{
		chat := api.Group("/chat")
		{
			chat.POST("/completions", h.completions)
			chat.POST("/delete", h.deleteConversation)
		}
		file := api.Group("/file")
		{
			file.POST("/upload", h.upload)
		}
	}
}

type handler struct {
	service *doubao.Service
}

type errorStatus interface {
	StatusCode() int
}

type errorResponse struct {
	Error string `json:"error"`
}

func (h *handler) completions(c *gin.Context) {
	var req model.CompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	ctx := c.Request.Context()
	resp, err := h.service.ChatCompletion(ctx, req)
	if err != nil {
		renderError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *handler) deleteConversation(c *gin.Context) {
	conversationID := c.Query("conversation_id")
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "conversation_id is required"})
		return
	}

	ctx := c.Request.Context()
	res, err := h.service.DeleteConversation(ctx, conversationID)
	if err != nil {
		renderError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *handler) upload(c *gin.Context) {
	fileTypeStr := c.Query("file_type")
	fileName := c.Query("file_name")
	if fileTypeStr == "" || fileName == "" {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "file_type and file_name are required"})
		return
	}

	fileType, err := strconv.Atoi(fileTypeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "file_type must be an integer"})
		return
	}

	body, err := c.GetRawData()
	if err != nil {
		renderError(c, err)
		return
	}

	ctx := c.Request.Context()
	resp, err := h.service.UploadFile(ctx, fileType, fileName, body)
	if err != nil {
		renderError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func renderError(c *gin.Context, err error) {
	var httpErr *model.HTTPError
	status := http.StatusInternalServerError
	if errors.As(err, &httpErr) {
		status = httpErr.StatusCode()
	} else if es, ok := err.(errorStatus); ok {
		status = es.StatusCode()
	}
	if status == 0 {
		status = http.StatusInternalServerError
	}
	c.JSON(status, errorResponse{Error: err.Error()})
}

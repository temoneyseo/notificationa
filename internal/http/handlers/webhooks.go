package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/user/notification-hub/internal/domain"
	"github.com/user/notification-hub/internal/security"
	"github.com/user/notification-hub/internal/service"
)

type WebhookHandler struct {
	webhooks service.WebhookStore
	cipher   *security.Cipher
}

type webhookRequest struct {
	URL      string   `json:"url" binding:"required"`
	Events   []string `json:"events"`
	Secret   string   `json:"secret"`
	IsActive *bool    `json:"is_active"`
}

func NewWebhookHandler(webhooks service.WebhookStore, cipher *security.Cipher) *WebhookHandler {
	return &WebhookHandler{webhooks: webhooks, cipher: cipher}
}

func (h *WebhookHandler) Create(c *gin.Context) {
	var req webhookRequest
	if !bindJSON(c, &req) {
		return
	}
	hook, err := h.webhookFromRequest(req)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.webhooks.Create(c.Request.Context(), hook); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusCreated, maskWebhook(*hook))
}

func (h *WebhookHandler) List(c *gin.Context) {
	items, err := h.webhooks.List(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]domain.WebhookConfig, 0, len(items))
	for _, item := range items {
		out = append(out, maskWebhook(item))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (h *WebhookHandler) Get(c *gin.Context) {
	hook, err := h.webhooks.Get(c.Request.Context(), c.Param("id"))
	if notFoundIfNil(c, hook, err) {
		return
	}
	c.JSON(http.StatusOK, maskWebhook(*hook))
}

func (h *WebhookHandler) Update(c *gin.Context) {
	var req webhookRequest
	if !bindJSON(c, &req) {
		return
	}
	hook, err := h.webhooks.Get(c.Request.Context(), c.Param("id"))
	if notFoundIfNil(c, hook, err) {
		return
	}
	next, err := h.webhookFromRequest(req)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	next.ID = hook.ID
	next.CreatedAt = hook.CreatedAt
	next.LastTriggeredAt = hook.LastTriggeredAt
	next.LastError = hook.LastError
	if err := h.webhooks.Update(c.Request.Context(), next); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, maskWebhook(*next))
}

func (h *WebhookHandler) Delete(c *gin.Context) {
	if err := h.webhooks.Delete(c.Request.Context(), c.Param("id")); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *WebhookHandler) webhookFromRequest(req webhookRequest) (*domain.WebhookConfig, error) {
	hook := domain.NewWebhookConfig(req.URL, req.Events)
	if req.IsActive != nil {
		hook.IsActive = *req.IsActive
	}
	hook.Secret = req.Secret
	if h.cipher != nil && hook.Secret != "" {
		secret, err := h.cipher.EncryptString(hook.Secret)
		if err != nil {
			return nil, err
		}
		hook.Secret = secret
	}
	hook.Normalize()
	return hook, nil
}

func maskWebhook(hook domain.WebhookConfig) domain.WebhookConfig {
	if hook.Secret != "" {
		hook.Secret = "********"
	}
	return hook
}

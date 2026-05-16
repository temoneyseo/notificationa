package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/user/notification-hub/internal/security"
	"github.com/user/notification-hub/internal/service"
)

type Dependencies struct {
	Messages service.MessageStore
	Channels service.ChannelStore
	Webhooks service.WebhookStore
	Pipeline *service.Pipeline
	Cipher   *security.Cipher
}

func RegisterRoutes(router *gin.Engine, deps Dependencies) {
	health := NewHealthHandler()
	router.GET("/health", health.Get)

	v1 := router.Group("/api/v1")
	messages := NewMessageHandler(deps.Messages, deps.Channels, deps.Pipeline)
	v1.POST("/messages", messages.Create)
	v1.POST("/notify", messages.Notify)
	v1.GET("/messages/inbox", messages.Inbox)

	channels := NewChannelHandler(deps.Channels, deps.Cipher)
	v1.POST("/channels", channels.Create)
	v1.GET("/channels", channels.List)
	v1.GET("/channels/:id", channels.Get)
	v1.PUT("/channels/:id", channels.Update)
	v1.DELETE("/channels/:id", channels.Delete)

	webhooks := NewWebhookHandler(deps.Webhooks, deps.Cipher)
	v1.POST("/webhooks", webhooks.Create)
	v1.GET("/webhooks", webhooks.List)
	v1.GET("/webhooks/:id", webhooks.Get)
	v1.PUT("/webhooks/:id", webhooks.Update)
	v1.DELETE("/webhooks/:id", webhooks.Delete)
}

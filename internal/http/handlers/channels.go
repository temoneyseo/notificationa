package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/user/notification-hub/internal/domain"
	"github.com/user/notification-hub/internal/security"
	"github.com/user/notification-hub/internal/service"
)

type ChannelHandler struct {
	channels service.ChannelStore
}

func NewChannelHandler(channels service.ChannelStore, _ *security.Cipher) *ChannelHandler {
	return &ChannelHandler{channels: channels}
}

func (h *ChannelHandler) Create(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "channels are managed by config")
}

func (h *ChannelHandler) List(c *gin.Context) {
	items, err := h.channels.List(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]domain.Channel, 0, len(items))
	for _, item := range items {
		out = append(out, h.maskChannel(item))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (h *ChannelHandler) Get(c *gin.Context) {
	ch, err := h.channels.Get(c.Request.Context(), c.Param("id"))
	if notFoundIfNil(c, ch, err) {
		return
	}
	c.JSON(http.StatusOK, h.maskChannel(*ch))
}

func (h *ChannelHandler) Update(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "channels are managed by config")
}

func (h *ChannelHandler) Delete(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "channels are managed by config")
}

func (h *ChannelHandler) maskChannel(ch domain.Channel) domain.Channel {
	ch.Config = security.MaskConfig(ch.Config)
	return ch
}

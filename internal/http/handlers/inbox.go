package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/user/notification-hub/internal/storage/sqlite"
)

func (h *MessageHandler) Inbox(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	items, err := h.messages.ListInbox(c.Request.Context(), sqlite.MessageListOptions{
		Channel: c.Query("channel"),
		Source:  c.Query("source"),
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":   items,
		"limit":  limit,
		"offset": offset,
	})
}

package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

func respondError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

func notFoundIfNil(c *gin.Context, value any, err error) bool {
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return true
	}
	if value == nil {
		respondError(c, http.StatusNotFound, "not found")
		return true
	}
	return false
}

var errInvalidInput = errors.New("invalid input")

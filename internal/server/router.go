package server

import "github.com/gin-gonic/gin"

func NewRouter(deps Dependencies) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	if deps.RegisterHandlers != nil {
		deps.RegisterHandlers(router)
	}
	return router
}

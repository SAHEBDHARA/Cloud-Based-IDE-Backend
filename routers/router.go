package routers

import (
	"server/handlers"

	"github.com/gin-gonic/gin"
)

func AllRouter(r *gin.Engine) {
r.GET("/get/files", handlers.GetFiles)
r.GET("/get/file/content",handlers.GetFileContentHandler)
}

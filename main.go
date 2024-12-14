package main

import (
	"net/http"
	"server/handlers"
	"server/routers"

	"github.com/gin-gonic/gin"
)

func main() {
	
	router := gin.Default()
	// Enable CORS for all origins
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	router.GET("/", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"message": "the server is up and running..",
		})
	})
	// fileTree, err := generateFileTree("working_dir")
	// if err != nil {
	// 	log.Println(err)
	// 	return
	// }
	routers.AllRouter(router)

	// log.Println("this is the working dir",fileTree)
	router.GET("/ws", func(ctx *gin.Context) {
		handlers.HandleWebSocket(ctx.Writer, ctx.Request)
	})
	router.Run(":9000")
}

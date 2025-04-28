package main

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"com.example/relay/config"
	"com.example/relay/handlers"
)

func main() {
	// 初始化配置
	config.Init()

	router := gin.Default()

	// cors
	router.Use(cors.Default())

	// 增加最大请求体大小限制
	router.MaxMultipartMemory = 8 << 20 // 8 MiB

	router.GET("/", func(c *gin.Context) {
		c.String(200, "啥也没有😅!")
	})

	// 节点相关路由
	nodeRouter := router.Group("/node")
	handlers.SetupNodeRoutes(nodeRouter)

	// 文件相关路由
	fileRouter := router.Group("/file")
	handlers.SetupFileRoutes(fileRouter)

	// WebSocket相关路由
	socketRouter := router.Group("/socket")
	handlers.SetupSocketRoutes(socketRouter)

	// 同步相关路由
	syncRouter := router.Group("/sync")
	handlers.SetupSyncRoutes(syncRouter)

	err := router.Run(":8080")

	if err != nil {
		panic(err)
	}
}

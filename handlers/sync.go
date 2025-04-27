package handlers

import (
	"github.com/gin-gonic/gin"
)

// SetupSyncRoutes 设置同步相关路由
func SetupSyncRoutes(router *gin.RouterGroup) {
	router.POST("/sync", SyncData)
}

// SyncData 同步数据
func SyncData(c *gin.Context) {
	c.String(200, "<UNK>")
}

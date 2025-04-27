package handlers

import (
	"github.com/gin-gonic/gin"
)

// SetupNodeRoutes 设置节点相关路由
func SetupNodeRoutes(router *gin.RouterGroup) {
	// 注册节点
	router.POST("/register", RegisterNode)

	// 连接到 node relay 服务
	router.GET("/report", ReportNodeState)

}

// RegisterNode 注册节点
func RegisterNode(c *gin.Context) {
	c.String(200, "注册成功")
}

// ReportNodeState 报告节点状态
func ReportNodeState(c *gin.Context) {
	c.String(200, "连接成功")
}

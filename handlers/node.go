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

	// 初始化节点，由 manager 调用
	router.POST("/init/:id", InitNodeById)
}

// RegisterNode 注册节点
func RegisterNode(c *gin.Context) {
	c.String(200, "注册成功")
}

// ReportNodeState 报告节点状态
func ReportNodeState(c *gin.Context) {
	c.String(200, "连接成功")
}

// InitNode 初始化节点
func InitNodeById(c *gin.Context) {
	id := c.Param("id")
	c.String(200, "初始化成功，节点ID：%s", id)
}

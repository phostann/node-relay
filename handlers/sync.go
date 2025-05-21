package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"com.example/relay/config"
	"github.com/gin-gonic/gin"
)

// SetupSyncRoutes 设置同步相关路由
func SetupSyncRoutes(router *gin.RouterGroup) {
	router.POST("/sync/upload", SyncUpload)
	router.GET("/sync/download", SyncDownload)
	router.POST("/sync/complete", SyncComplete)
}

// SyncUpload 同步上传文件
func SyncUpload(c *gin.Context) {
	// 获取 node 信息
	uid := c.PostForm("uid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法获取同步的节点信息"})
		return
	}
	filename := c.PostForm("filename")
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法获取同步的资源名称"})
	}
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法获取上传的文件"})
		return
	}

	// 写入到 uploads/resource_name
	filePath := filepath.Join(config.UploadsDir, uid, filename)
	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存文件失败"})
		return
	}

	// 发送 {type: "sync", data: {uid: uid, resource_name: resource_name}}

	message := map[string]interface{}{
		"type": "sync",
		"data": map[string]string{
			"uid":      uid,
			"filename": filename,
		},
	}

	jsonBytes, err := json.Marshal(message)
	if err != nil {
		// 根据您的错误处理策略处理错误
		log.Printf("JSON marshal error: %v", err)
		return
	}

	wsManager.SendMessage(uid, jsonBytes)

	c.JSON(http.StatusOK, gin.H{"message": "同步请求已发送"})

}

// SyncDownload 同步下载文件
func SyncDownload(c *gin.Context) {
	filename := c.Query("filename")

	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法获取同步的资源名称"})
		return
	}

	filePath := filepath.Join(config.UploadsDir, filename)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	c.File(filePath)
}

// SyncComplete 同步完成，删除文件
func SyncComplete(c *gin.Context) {
	uid := c.PostForm("uid")
	filename := c.PostForm("filename")

	if uid == "" || filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法获取同步的节点信息或资源名称"})
		return
	}

	filePath := filepath.Join(config.UploadsDir, filename)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	os.Remove(filePath)
}

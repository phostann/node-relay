package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"com.example/relay/config"
	"com.example/relay/models"
	"com.example/relay/utils"
	"github.com/gin-gonic/gin"
)

// SetupFileRoutes 设置文件相关路由
func SetupFileRoutes(router *gin.RouterGroup) {
	// 初始化上传
	router.POST("/upload/init", InitUpload)

	// 上传文件分块
	router.POST("/upload/chunk", UploadChunk)

	// 完成上传，合并文件
	router.POST("/upload/complete", CompleteUpload)

	// 查询上传状态
	router.GET("/upload/status", CheckUploadStatus)

	// 原有的上传路由，保留为简单上传入口
	router.POST("/upload", SimpleUpload)
}

// SimpleUpload 简单上传处理
func SimpleUpload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "上传文件时出错: " + err.Error(),
		})
		return
	}

	// 限制简单上传的文件大小
	if file.Size > 10<<20 { // 10 MiB
		c.JSON(http.StatusBadRequest, gin.H{
			"error":       "文件过大，请使用分块上传接口",
			"maxSize":     "10MB",
			"currentSize": fmt.Sprintf("%.2f MB", float64(file.Size)/(1024*1024)),
		})
		return
	}

	dst := filepath.Join(config.UploadsDir, file.Filename)

	// 保存文件
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "保存文件失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "文件上传成功",
		"file":    file.Filename,
		"size":    file.Size,
	})
}

// InitUpload 初始化上传请求
func InitUpload(c *gin.Context) {
	fileName := c.PostForm("fileName")
	fileSizeStr := c.PostForm("fileSize")
	chunkSizeStr := c.PostForm("chunkSize")

	if fileName == "" || fileSizeStr == "" || chunkSizeStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "参数不完整",
		})
		return
	}

	fileSize, err := strconv.ParseInt(fileSizeStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "fileSize 参数格式不正确",
		})
		return
	}

	chunkSize, err := strconv.ParseInt(chunkSizeStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "chunkSize 参数格式不正确",
		})
		return
	}

	// 生成文件唯一标识
	fileID := utils.GenerateFileID(fileName, fileSize)

	// 计算总块数
	totalChunks := int((fileSize + chunkSize - 1) / chunkSize)

	// 创建上传信息
	models.UploadsMutex.Lock()
	defer models.UploadsMutex.Unlock()

	// 检查是否已存在上传任务
	if info, exists := models.Uploads[fileID]; exists {
		c.JSON(http.StatusOK, gin.H{
			"fileID":      fileID,
			"totalChunks": info.TotalChunks,
			"chunkSize":   info.ChunkSize,
			"completed":   models.CountCompletedChunks(info.Completed),
			"resumed":     true,
		})
		return
	}

	// 创建新的上传任务
	models.Uploads[fileID] = &models.UploadInfo{
		FileID:      fileID,
		FileName:    fileName,
		TotalChunks: totalChunks,
		TotalSize:   fileSize,
		ChunkSize:   chunkSize,
		Completed:   make([]bool, totalChunks),
	}

	c.JSON(http.StatusOK, gin.H{
		"fileID":      fileID,
		"totalChunks": totalChunks,
		"chunkSize":   chunkSize,
		"resumed":     false,
	})
}

// UploadChunk 上传文件分块
func UploadChunk(c *gin.Context) {
	fileID := c.PostForm("fileID")
	chunkIndexStr := c.PostForm("chunkIndex")

	if fileID == "" || chunkIndexStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "参数不完整",
		})
		return
	}

	// 检查上传信息是否存在
	models.UploadsMutex.Lock()
	uploadInfo, exists := models.Uploads[fileID]
	models.UploadsMutex.Unlock()

	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的 fileID，请先初始化上传",
		})
		return
	}

	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "chunkIndex 参数格式不正确",
		})
		return
	}

	// 检查块索引是否有效
	if chunkIndex < 0 || chunkIndex >= uploadInfo.TotalChunks {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "无效的块索引",
			"validRange": fmt.Sprintf("0-%d", uploadInfo.TotalChunks-1),
		})
		return
	}

	// 检查分块是否已上传
	uploadInfo.Mu.Lock()
	if uploadInfo.Completed[chunkIndex] {
		uploadInfo.Mu.Unlock()
		c.JSON(http.StatusOK, gin.H{
			"message":    "分块已上传",
			"chunkIndex": chunkIndex,
		})
		return
	}
	uploadInfo.Mu.Unlock()

	// 获取上传文件
	file, err := c.FormFile("chunk")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "获取文件数据失败: " + err.Error(),
		})
		return
	}

	// 创建临时文件路径
	chunkPath := filepath.Join(config.TempDir, fmt.Sprintf("%s-%d", fileID, chunkIndex))

	// 保存分块文件
	if err := c.SaveUploadedFile(file, chunkPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "保存分块文件失败: " + err.Error(),
		})
		return
	}

	// 更新分块状态
	uploadInfo.Mu.Lock()
	uploadInfo.Completed[chunkIndex] = true
	completed := models.CountCompletedChunks(uploadInfo.Completed)
	uploadInfo.Mu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"message":    "分块上传成功",
		"chunkIndex": chunkIndex,
		"completed":  completed,
		"total":      uploadInfo.TotalChunks,
	})
}

// CompleteUpload 完成上传，合并文件
func CompleteUpload(c *gin.Context) {
	fileID := c.PostForm("fileID")

	if fileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "参数不完整",
		})
		return
	}

	// 检查上传信息是否存在
	models.UploadsMutex.Lock()
	uploadInfo, exists := models.Uploads[fileID]
	models.UploadsMutex.Unlock()

	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的 fileID",
		})
		return
	}

	// 检查所有分块是否已上传
	uploadInfo.Mu.Lock()
	for i, completed := range uploadInfo.Completed {
		if !completed {
			uploadInfo.Mu.Unlock()
			c.JSON(http.StatusBadRequest, gin.H{
				"error":      "有分块尚未上传完成",
				"chunkIndex": i,
			})
			return
		}
	}
	uploadInfo.Mu.Unlock()

	// 合并文件
	finalPath := filepath.Join(config.UploadsDir, uploadInfo.FileName)
	finalFile, err := os.Create(finalPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "创建最终文件失败: " + err.Error(),
		})
		return
	}
	defer finalFile.Close()

	// 逐个合并分块
	for i := 0; i < uploadInfo.TotalChunks; i++ {
		chunkPath := filepath.Join(config.TempDir, fmt.Sprintf("%s-%d", fileID, i))
		chunkFile, err := os.Open(chunkPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("打开分块文件失败: %s", err.Error()),
			})
			return
		}

		_, err = io.Copy(finalFile, chunkFile)
		chunkFile.Close()

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("合并分块文件失败: %s", err.Error()),
			})
			return
		}

		// 删除临时分块文件
		os.Remove(chunkPath)
	}

	// 清理上传信息
	models.UploadsMutex.Lock()
	delete(models.Uploads, fileID)
	models.UploadsMutex.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"message":  "文件上传完成",
		"fileName": uploadInfo.FileName,
		"fileSize": uploadInfo.TotalSize,
		"filePath": finalPath,
	})
}

// CheckUploadStatus 查询上传状态
func CheckUploadStatus(c *gin.Context) {
	fileID := c.Query("fileID")

	if fileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "参数不完整",
		})
		return
	}

	// 检查上传信息是否存在
	models.UploadsMutex.Lock()
	uploadInfo, exists := models.Uploads[fileID]
	models.UploadsMutex.Unlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "找不到上传任务",
		})
		return
	}

	// 计算已完成的分块
	uploadInfo.Mu.Lock()
	completedChunks := models.CountCompletedChunks(uploadInfo.Completed)
	uploadInfo.Mu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"fileID":      fileID,
		"fileName":    uploadInfo.FileName,
		"totalChunks": uploadInfo.TotalChunks,
		"completed":   completedChunks,
		"percentage":  float64(completedChunks) / float64(uploadInfo.TotalChunks) * 100,
	})
}

package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

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

	// 新增下载相关路由
	// 简单下载接口
	router.GET("/download", SimpleDownload)

	// 初始化大文件下载
	router.GET("/download/init", InitDownload)

	// 分块下载接口
	router.GET("/download/chunk", DownloadChunk)

	// 查询下载元信息
	router.GET("/download/info", GetDownloadInfo)
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
			"error":        "文件过大，请使用分块上传接口",
			"max_size":     "10MB",
			"current_size": fmt.Sprintf("%.2f MB", float64(file.Size)/(1024*1024)),
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
	fileName := c.PostForm("file_name")
	fileSizeStr := c.PostForm("file_size")
	chunkSizeStr := c.PostForm("chunk_size")
	fileHash := c.PostForm("file_hash") // 接收文件哈希值，MD5

	if fileName == "" || fileSizeStr == "" || chunkSizeStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "参数不完整",
		})
		return
	}

	fileSize, err := strconv.ParseInt(fileSizeStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "file_size 参数格式不正确",
		})
		return
	}

	chunkSize, err := strconv.ParseInt(chunkSizeStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "chunk_size 参数格式不正确",
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
			"file_id":      fileID,
			"total_chunks": info.TotalChunks,
			"chunk_size":   info.ChunkSize,
			"completed":    models.CountCompletedChunks(info.Completed),
			"file_hash":    info.FileHash,
			"resumed":      true,
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
		FileHash:    fileHash,
		ChunkHashes: make(map[int]string),
	}

	c.JSON(http.StatusOK, gin.H{
		"file_id":      fileID,
		"total_chunks": totalChunks,
		"chunk_size":   chunkSize,
		"file_hash":    fileHash,
		"resumed":      false,
	})
}

// UploadChunk 上传文件分块
func UploadChunk(c *gin.Context) {
	fileID := c.PostForm("file_id")
	chunkIndexStr := c.PostForm("chunk_index")
	chunkHash := c.PostForm("chunk_hash") // 接收分块哈希值

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
			"error": "无效的 file_id，请先初始化上传",
		})
		return
	}

	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "chunk_index 参数格式不正确",
		})
		return
	}

	// 检查块索引是否有效
	if chunkIndex < 0 || chunkIndex >= uploadInfo.TotalChunks {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":       "无效的块索引",
			"valid_range": fmt.Sprintf("0-%d", uploadInfo.TotalChunks-1),
		})
		return
	}

	// 检查分块是否已上传
	uploadInfo.Mu.Lock()
	if uploadInfo.Completed[chunkIndex] {
		uploadInfo.Mu.Unlock()
		c.JSON(http.StatusOK, gin.H{
			"message":     "分块已上传",
			"chunk_index": chunkIndex,
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

	// 如果提供了分块哈希值，验证分块完整性
	if chunkHash != "" {
		// 计算保存的分块文件哈希值 - 使用MD5而非SHA256
		calculatedHash, err := utils.CalculateChunkMD5(chunkPath)
		if err != nil {
			os.Remove(chunkPath) // 删除有问题的文件
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "计算分块哈希值失败: " + err.Error(),
			})
			return
		}

		// 比较哈希值
		if calculatedHash != chunkHash {
			os.Remove(chunkPath) // 删除不匹配的文件
			c.JSON(http.StatusBadRequest, gin.H{
				"error":           "分块完整性验证失败",
				"expected_hash":   chunkHash,
				"calculated_hash": calculatedHash,
				"chunk_index":     chunkIndex,
			})
			return
		}

		// 保存分块哈希值
		uploadInfo.Mu.Lock()
		uploadInfo.ChunkHashes[chunkIndex] = calculatedHash
		uploadInfo.Mu.Unlock()
	}

	// 更新分块状态
	uploadInfo.Mu.Lock()
	uploadInfo.Completed[chunkIndex] = true
	completed := models.CountCompletedChunks(uploadInfo.Completed)
	uploadInfo.Mu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"message":     "分块上传成功",
		"chunk_index": chunkIndex,
		"completed":   completed,
		"total":       uploadInfo.TotalChunks,
		"verified":    chunkHash != "",
	})
}

// CompleteUpload 完成上传，合并文件
func CompleteUpload(c *gin.Context) {
	fileID := c.PostForm("file_id")

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
			"error": "无效的 file_id",
		})
		return
	}

	// 检查所有分块是否已上传
	uploadInfo.Mu.Lock()
	for i, completed := range uploadInfo.Completed {
		if !completed {
			uploadInfo.Mu.Unlock()
			c.JSON(http.StatusBadRequest, gin.H{
				"error":       "有分块尚未上传完成",
				"chunk_index": i,
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

	// 校验合并后的文件完整性（如果初始化时提供了文件哈希）
	fileIntegrityVerified := false
	var calculatedFileHash string

	if uploadInfo.FileHash != "" {
		// 计算合并后的文件哈希值 - 使用MD5而非SHA256
		calculatedFileHash, err = utils.CalculateFileMD5(finalPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "计算文件哈希值失败: " + err.Error(),
				"note":  "文件已合并，但未能验证完整性",
			})
			return
		}

		// 比较哈希值
		if calculatedFileHash != uploadInfo.FileHash {
			// 哈希不匹配，但文件已合并，通知客户端验证失败
			c.JSON(http.StatusOK, gin.H{
				"message":          "文件已合并，但完整性验证失败",
				"file_name":        uploadInfo.FileName,
				"file_size":        uploadInfo.TotalSize,
				"file_path":        finalPath,
				"expected_hash":    uploadInfo.FileHash,
				"calculated_hash":  calculatedFileHash,
				"integrity_status": "failed",
			})

			// 清理上传信息
			models.UploadsMutex.Lock()
			delete(models.Uploads, fileID)
			models.UploadsMutex.Unlock()

			return
		}

		fileIntegrityVerified = true
	}

	// 清理上传信息
	models.UploadsMutex.Lock()
	delete(models.Uploads, fileID)
	models.UploadsMutex.Unlock()

	// 构建响应
	response := gin.H{
		"message":   "文件上传完成",
		"file_name": uploadInfo.FileName,
		"file_size": uploadInfo.TotalSize,
		"file_path": finalPath,
	}

	// 如果进行了完整性校验，添加相关信息
	if uploadInfo.FileHash != "" {
		response["integrity_verified"] = fileIntegrityVerified
		response["file_hash"] = calculatedFileHash
	}

	c.JSON(http.StatusOK, response)
}

// CheckUploadStatus 查询上传状态
func CheckUploadStatus(c *gin.Context) {
	fileID := c.Query("file_id")

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
		"file_id":      fileID,
		"file_name":    uploadInfo.FileName,
		"total_chunks": uploadInfo.TotalChunks,
		"completed":    completedChunks,
		"percentage":   float64(completedChunks) / float64(uploadInfo.TotalChunks) * 100,
	})
}

// SimpleDownload 简单文件下载处理
func SimpleDownload(c *gin.Context) {
	fileName := c.Query("file_name")
	if fileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "参数不完整，请提供文件名",
		})
		return
	}

	// 构建文件路径
	filePath := filepath.Join(config.UploadsDir, fileName)

	// 检查文件是否存在
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "找不到指定文件",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "获取文件信息失败: " + err.Error(),
			})
		}
		return
	}

	// 如果文件比较小，直接下载
	if fileInfo.Size() <= 10<<20 { // 10 MiB
		c.File(filePath)
		return
	}

	// 对于大文件，建议使用分块下载
	fileID := utils.GenerateFileID(fileName, fileInfo.Size())
	c.JSON(http.StatusOK, gin.H{
		"message":           "文件过大，建议使用分块下载接口",
		"file_id":           fileID,
		"file_name":         fileName,
		"file_size":         fileInfo.Size(),
		"download_init_url": fmt.Sprintf("/file/download/init?file_name=%s", fileName),
	})
}

// InitDownload 初始化大文件下载
func InitDownload(c *gin.Context) {
	fileName := c.Query("file_name")
	chunkSizeStr := c.DefaultQuery("chunk_size", "1048576") // 默认1MB块大小

	if fileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "参数不完整，请提供文件名",
		})
		return
	}

	// 解析分块大小
	chunkSize, err := strconv.ParseInt(chunkSizeStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "chunk_size 参数格式不正确",
		})
		return
	}

	// 构建文件路径
	filePath := filepath.Join(config.UploadsDir, fileName)

	// 检查文件是否存在
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "找不到指定文件",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "获取文件信息失败: " + err.Error(),
			})
		}
		return
	}

	// 计算文件总块数
	fileSize := fileInfo.Size()
	totalChunks := int((fileSize + chunkSize - 1) / chunkSize)

	// 生成文件唯一标识
	fileID := utils.GenerateFileID(fileName, fileSize)

	// 计算文件哈希值 - 使用MD5而非SHA256
	fileHash, err := utils.CalculateFileMD5(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "计算文件哈希值失败: " + err.Error(),
		})
		return
	}

	// 预计算所有分块的哈希值
	chunkHashes := make(map[int]string)

	// 保存下载信息
	downloadInfo := &models.DownloadInfo{
		FileID:      fileID,
		FileName:    fileName,
		FilePath:    filePath,
		TotalSize:   fileSize,
		ChunkSize:   chunkSize,
		TotalChunks: totalChunks,
		CreatedAt:   time.Now(),
		FileHash:    fileHash,
		ChunkHashes: chunkHashes,
	}

	models.SaveDownloadInfo(downloadInfo)

	// 启动后台协程计算每个分块的哈希值
	go calculateChunkHashes(downloadInfo)

	// 返回下载初始化信息
	c.JSON(http.StatusOK, gin.H{
		"file_id":      fileID,
		"file_name":    fileName,
		"file_size":    fileSize,
		"chunk_size":   chunkSize,
		"total_chunks": totalChunks,
		"file_hash":    fileHash,
		"download_url": fmt.Sprintf("/file/download/chunk?file_id=%s&chunk_index=", fileID),
	})
}

// calculateChunkHashes 计算文件所有分块的哈希值（作为后台任务运行）
func calculateChunkHashes(info *models.DownloadInfo) {
	file, err := os.Open(info.FilePath)
	if err != nil {
		fmt.Printf("打开文件失败: %s\n", err.Error())
		return
	}
	defer file.Close()

	buffer := make([]byte, info.ChunkSize)

	for i := 0; i < info.TotalChunks; i++ {
		// 定位到对应块的位置
		offset := int64(i) * info.ChunkSize
		_, err := file.Seek(offset, io.SeekStart)
		if err != nil {
			fmt.Printf("定位到分块位置失败: %s\n", err.Error())
			continue
		}

		// 读取块数据
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			fmt.Printf("读取分块数据失败: %s\n", err.Error())
			continue
		}

		// 计算哈希值 - 使用MD5而非SHA256
		hash := utils.CalculateDataMD5(buffer[:n])

		// 保存分块哈希值
		info.Mu.Lock()
		info.ChunkHashes[i] = hash
		info.Mu.Unlock()
	}
}

// DownloadChunk 下载文件分块
func DownloadChunk(c *gin.Context) {
	fileID := c.Query("file_id")
	chunkIndexStr := c.Query("chunk_index")

	if fileID == "" || chunkIndexStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "参数不完整",
		})
		return
	}

	// 获取下载信息
	downloadInfo, exists := models.GetDownloadInfo(fileID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "无效的文件ID，请先初始化下载",
		})
		return
	}

	// 解析块索引
	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "chunk_index 参数格式不正确",
		})
		return
	}

	// 检查块索引是否有效
	if chunkIndex < 0 || chunkIndex >= downloadInfo.TotalChunks {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":       "无效的块索引",
			"valid_range": fmt.Sprintf("0-%d", downloadInfo.TotalChunks-1),
		})
		return
	}

	// 计算块的起始和结束位置
	start := int64(chunkIndex) * downloadInfo.ChunkSize
	end := start + downloadInfo.ChunkSize
	if end > downloadInfo.TotalSize {
		end = downloadInfo.TotalSize
	}

	// 打开文件
	file, err := os.Open(downloadInfo.FilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "打开文件失败: " + err.Error(),
		})
		return
	}
	defer file.Close()

	// 获取分块哈希值（如果已计算）
	downloadInfo.Mu.Lock()
	chunkHash, hashExists := downloadInfo.ChunkHashes[chunkIndex]
	downloadInfo.Mu.Unlock()

	// 设置响应头
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s-part%d", downloadInfo.FileName, chunkIndex))
	c.Header("Content-Length", fmt.Sprintf("%d", end-start))
	c.Header("Accept-Ranges", "bytes")
	c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end-1, downloadInfo.TotalSize))

	// 如果存在分块哈希值，添加到响应头
	if hashExists {
		c.Header("X-Chunk-Hash", chunkHash)
	}

	// 设置状态码为206（部分内容）
	c.Status(http.StatusPartialContent)

	// 定位到文件的指定位置
	_, err = file.Seek(start, io.SeekStart)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "文件定位失败: " + err.Error(),
		})
		return
	}

	// 创建一个限制读取大小的Reader
	limitReader := io.LimitReader(file, end-start)

	// 将数据直接写入响应
	_, err = io.Copy(c.Writer, limitReader)
	if err != nil {
		// 这里不需要返回错误，因为响应已经开始写入
		fmt.Printf("发送文件块失败: %s\n", err.Error())
	}
}

// GetDownloadInfo 获取下载信息
func GetDownloadInfo(c *gin.Context) {
	fileID := c.Query("file_id")

	if fileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "参数不完整",
		})
		return
	}

	// 获取下载信息
	downloadInfo, exists := models.GetDownloadInfo(fileID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "找不到下载信息",
		})
		return
	}

	// 计算已完成计算哈希的分块数量
	downloadInfo.Mu.Lock()
	hashCompletedChunks := len(downloadInfo.ChunkHashes)
	downloadInfo.Mu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"file_id":               fileID,
		"file_name":             downloadInfo.FileName,
		"file_size":             downloadInfo.TotalSize,
		"chunk_size":            downloadInfo.ChunkSize,
		"total_chunks":          downloadInfo.TotalChunks,
		"created_at":            downloadInfo.CreatedAt,
		"file_hash":             downloadInfo.FileHash,
		"hash_completed_chunks": hashCompletedChunks,
		"hash_progress":         float64(hashCompletedChunks) / float64(downloadInfo.TotalChunks) * 100,
		"download_url":          fmt.Sprintf("/file/download/chunk?file_id=%s&chunk_index=", fileID),
	})
}

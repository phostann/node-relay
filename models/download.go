package models

import (
	"sync"
	"time"
)

// DownloadInfo 下载信息结构体
type DownloadInfo struct {
	FileID      string         // 文件唯一标识
	FileName    string         // 文件名
	FilePath    string         // 文件路径
	TotalSize   int64          // 文件总大小
	ChunkSize   int64          // 每个块的大小
	TotalChunks int            // 总块数
	CreatedAt   time.Time      // 创建时间
	FileHash    string         // 文件的哈希值
	ChunkHashes map[int]string // 分块哈希值映射
	Mu          sync.Mutex
}

// Downloads 全局下载信息记录
var Downloads = make(map[string]*DownloadInfo)
var DownloadsMutex sync.Mutex

// GetDownloadInfo 获取下载信息
func GetDownloadInfo(fileID string) (*DownloadInfo, bool) {
	DownloadsMutex.Lock()
	defer DownloadsMutex.Unlock()

	info, exists := Downloads[fileID]
	return info, exists
}

// SaveDownloadInfo 保存下载信息
func SaveDownloadInfo(info *DownloadInfo) {
	DownloadsMutex.Lock()
	defer DownloadsMutex.Unlock()

	Downloads[info.FileID] = info
}

// RemoveDownloadInfo 删除下载信息
func RemoveDownloadInfo(fileID string) {
	DownloadsMutex.Lock()
	defer DownloadsMutex.Unlock()

	delete(Downloads, fileID)
}

// CleanupExpiredDownloads 清理过期的下载信息
func CleanupExpiredDownloads(maxAge time.Duration) {
	DownloadsMutex.Lock()
	defer DownloadsMutex.Unlock()

	now := time.Now()
	for id, info := range Downloads {
		if now.Sub(info.CreatedAt) > maxAge {
			delete(Downloads, id)
		}
	}
}

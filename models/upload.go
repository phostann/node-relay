package models

import (
	"sync"
)

// UploadInfo 上传信息结构体
type UploadInfo struct {
	FileID      string // 文件唯一标识
	FileName    string // 文件名
	TotalChunks int    // 总块数
	TotalSize   int64  // 文件总大小
	ChunkSize   int64  // 每个块的大小
	Completed   []bool // 已完成的块
	Mu          sync.Mutex
}

// Uploads 全局上传信息记录
var Uploads = make(map[string]*UploadInfo)
var UploadsMutex sync.Mutex

// CountCompletedChunks 计算已完成的分块数
func CountCompletedChunks(completed []bool) int {
	count := 0
	for _, v := range completed {
		if v {
			count++
		}
	}
	return count
}

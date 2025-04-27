package config

import (
	"os"
)

// 定义上传文件存储目录
const (
	UploadsDir = "./uploads"
	TempDir    = "./uploads/temp"
)

// Init 初始化配置
func Init() {
	// 确保上传目录存在
	os.MkdirAll(UploadsDir, 0755)
	os.MkdirAll(TempDir, 0755)
}

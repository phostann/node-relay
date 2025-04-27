package utils

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"strconv"
	"time"
)

// GenerateFileID 生成文件唯一标识
func GenerateFileID(fileName string, fileSize int64) string {
	h := md5.New()
	io.WriteString(h, fileName)
	io.WriteString(h, strconv.FormatInt(fileSize, 10))
	io.WriteString(h, strconv.FormatInt(time.Now().UnixNano(), 10))
	return hex.EncodeToString(h.Sum(nil))
}

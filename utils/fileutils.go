package utils

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
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

// CalculateFileSHA256 计算文件的SHA256哈希值
func CalculateFileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// CalculateDataSHA256 计算数据的SHA256哈希值
func CalculateDataSHA256(data []byte) string {
	hash := sha256.New()
	hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
}

// CalculateChunkSHA256 计算文件块的SHA256哈希值
func CalculateChunkSHA256(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return CalculateDataSHA256(data), nil
}

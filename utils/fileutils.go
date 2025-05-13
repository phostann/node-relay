package utils

import (
	"crypto/md5"
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

// CalculateFileMD5 计算文件的MD5哈希值 (与SparkMD5兼容)
func CalculateFileMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	buf := make([]byte, 4*1024*1024) // 4MB缓冲区，可按需调整

	for {
		n, err := file.Read(buf)
		if n > 0 {
			hash.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// CalculateDataMD5 计算数据的MD5哈希值
func CalculateDataMD5(data []byte) string {
	hash := md5.New()
	hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
}

// CalculateChunkMD5 计算文件块的MD5哈希值
func CalculateChunkMD5(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return CalculateDataMD5(data), nil
}

// 以下为向后兼容的函数，只是调用相应的MD5函数，用于平滑过渡

// CalculateFileSHA256 已替换为MD5实现，保持API兼容
func CalculateFileSHA256(filePath string) (string, error) {
	return CalculateFileMD5(filePath)
}

// CalculateDataSHA256 已替换为MD5实现，保持API兼容
func CalculateDataSHA256(data []byte) string {
	return CalculateDataMD5(data)
}

// CalculateChunkSHA256 已替换为MD5实现，保持API兼容
func CalculateChunkSHA256(filePath string) (string, error) {
	return CalculateChunkMD5(filePath)
}

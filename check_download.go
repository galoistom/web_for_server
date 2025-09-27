package main

import (
	//	"archive/tar"
	//	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// downloadFile 将 URL 内容下载到指定的本地路径
func DownloadFile(filepath string, url string) error {
	var check string
	fmt.Println("do you need to download it if from github? (yes/no):")
	fmt.Scanf("%s", &check)
	if check != "yes" {
		return fmt.Errorf("User stopped")
	}
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP StatusCode ERROR: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write: %w", err)
	}
	return nil
}

// untarGz 解压 .tar.gz 文件到指定的目录
// 它返回解包后创建的顶级目录名
/*
func UntarGz(srcPath string, destDir string) (string, error) {
	file, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("creating gzip reader failed: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	// 用于存储解包后得到的根目录名
	var rootDirName string

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // 循环结束
		}
		if err != nil {
			return "", fmt.Errorf("failed to read the head of tar: %w", err)
		}

		targetPath := filepath.Join(destDir, header.Name)

		// 记录第一个非隐藏目录，作为根目录名
		if rootDirName == "" && header.Typeflag == tar.TypeDir && !isDotFile(header.Name) {
			rootDirName = header.Name
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// 创建目录
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return "", fmt.Errorf("创建目录失败: %w", err)
			}
		case tar.TypeReg:
			// 创建文件并写入内容
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return "", fmt.Errorf("创建文件失败: %w", err)
			}
			defer outFile.Close()
			if _, err := io.Copy(outFile, tarReader); err != nil {
				return "", fmt.Errorf("写入文件内容失败: %w", err)
			}
		default:
			// 忽略其他文件类型（如符号链接等）
		}
	}
	return rootDirName, nil
}

func isDotFile(name string) bool {
	// 检查是否是隐藏文件或目录 (. 或 ..)
	return name == "." || name == ".." || len(name) > 0 && name[0] == '.'
}
*/

func CheckExist(name string) (bool, error) {
	_, err := os.Stat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		} else {
			return false, errors.New("failed to get file status")
		}
	}
	return true, nil
}

func DownloadFileToDir(dirPath string, fileName string, fileURL string) error {
	// --- 1. 确保目标目录存在 (最推荐的做法是直接使用 MkdirAll) ---
	// MkdirAll 会创建所有必要的父目录，如果目录已存在则不返回错误。
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to make folder %s: %w", dirPath, err)
	}
	fmt.Printf("folder '%s' is ready \n", dirPath)

	// --- 2. 拼接完整的本地文件路径 ---
	localPath := filepath.Join(dirPath, fileName)
	return DownloadFile(localPath, fileURL)

	//return nil

}

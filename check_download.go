package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

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
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to make folder %s: %w", dirPath, err)
	}
	fmt.Printf("folder '%s' is ready \n", dirPath)

	localPath := filepath.Join(dirPath, fileName)
	if b, err := CheckExist(localPath); !b && err != nil {
		return DownloadFile(localPath, fileURL)
	}

	return nil

}

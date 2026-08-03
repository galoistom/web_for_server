package main

import (
	"bufio"
	"os"
	"path/filepath"
	"sync"
)

type FileConfig struct {
	NAME    string `json:"name"`
	PATH    string `json:"path"`
	PREVIEW bool   `json:"preview"`
}

type File struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Type    string `json:"type"`
	Preview bool   `json:"preview"`
}

type FileManager struct {
	handlers map[string]File
	mu       sync.RWMutex
}

func goTail(filename string, n int) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		err := scanner.Err()
		if err != nil {
			return []string{}, err
		}
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	return lines, nil
}

func (f FileConfig) newFile() *File {
	f.PATH = os.ExpandEnv(f.PATH)
	return &File{
		ID:      filepath.Base(f.PATH),
		Name:    f.NAME,
		Path:    f.PATH,
		Type:    getFileType(f.PATH),
		Preview: f.PREVIEW,
	}
}

func getFileType(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	default:
		return "text"
	}
}

func (f *File) ReadRaw() (string, error) {
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (f *File) SaveRaw(content string) error {
	return os.WriteFile(f.Path, []byte(content), 0644)
}

func NewFileManager() *FileManager {
	return &FileManager{
		handlers: make(map[string]File),
	}
}

func (m *FileManager) RegisterHandler(id string, handler File) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[id] = handler
}

func (m *FileManager) GetFile(id string) (File, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h, ok := m.handlers[id]
	return h, ok
}

func (m *FileManager) ListFiles() []File {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]File, 0, len(m.handlers))
	for _, h := range m.handlers {
		result = append(result, h)
	}
	return result
}

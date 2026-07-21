package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gorcon/rcon"
)

type Config struct {
	RCON_HOST     string       `json:"rcon_host"`
	RCON_PASSWORD string       `json:"rcon_password"`
	PORT          string       `json:"port"`
	SERVER_PATH   string       `json:"server_path"`
	SHOW_LOG      bool         `json:"show_log"`
	START_COMMAND string       `json:"start_command"`
	FILES         []FileConfig `json:"files"`
}

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

var (
	mcServerCmd        *exec.Cmd
	mu                 sync.Mutex
	webConfig          Config
	configFilePosition string
	//go:embed static/*
	indexDir        embed.FS
	DEBUG           bool
	editConfigFiles *FileManager
)

// to check whether the serer started
func checkStarted() bool {
	if mcServerCmd != nil {
		return true
	}
	return false
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

func (m *FileManager) GetHandler(id string) (File, bool) {
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

// the function handleing the command from terminal(might not be used for a while)
func startCli() {
	go func() {
		fmt.Println("Type 'exit' to quit the application.")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			command := strings.TrimSpace(scanner.Text())
			switch command {
			case "exit":
				{
					log.Println("Exiting application...")
					if checkStarted() {
						resp, err := http.Get("http://127.0.0.1:" + webConfig.PORT + "/api/stop")
						if err != nil {
							fmt.Println("unable to connect to server", err)
						} else {
							log.Println("server closed")
							resp.Body.Close()
						}
					}
					os.Exit(0)
				} // 安全退出程序
			case "start":
				{
					if checkStarted() {
						log.Println("Server is already started...")
						continue
					}
					resp, err := http.Get("http://127.0.0.1:" + webConfig.PORT + "/api/start")
					if err != nil {
						fmt.Println("Error start server", err)
					} else {
						resp.Body.Close()
					}
				}
			case "reload":
				{
					loadConfig(configFilePosition)
					// log.Println(configFilePosition)
				}
			case "help":
				{
					fmt.Println("\"start\" to start the minecraft server")
					fmt.Println("\"stop\" to stop the minecraft server")
					fmt.Println("\"exit\" to exit the program")
					fmt.Println("\"help\" to get help for the program")
					fmt.Println("all other cases are send directly to the minecraft server as command")
				}
			default:
				{
					if !checkStarted() {
						fmt.Println("the server is not started yet")
						continue
					}
					conn, err := rcon.Dial(webConfig.RCON_HOST, webConfig.RCON_PASSWORD)
					if err != nil {
						fmt.Println("Unable to connect to server", err)
						continue
					}
					defer conn.Close()
					resp, err := conn.Execute(command)
					if err != nil {
						fmt.Println("failed to send the order", err)
					} else {
						fmt.Println(resp)
					}
				}
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "Error reading from stdin: %v\n", err)
			os.Exit(1)
		}
	}()
}

func handlecheckStart(w http.ResponseWriter, r *http.Request) {
	if checkStarted() {
		w.Write([]byte("running"))
	} else {
		w.Write([]byte("stopped"))
	}
}

func handleStart(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	if checkStarted() {
		http.Error(w, "Minecraft server is already running.", http.StatusConflict)
		return
	}

	cmd := exec.Command("sh", "-c", webConfig.START_COMMAND)
	cmd.Dir = os.ExpandEnv(webConfig.SERVER_PATH)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("failed to get out pipe:%v", err)
	}

	err = cmd.Start()
	if err != nil {
		log.Printf("failed to start server process: %v", err)
		http.Error(w, fmt.Sprintf("Failed to start server process: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Server started"))
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer stdout.Close()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Printf("%s\n", scanner.Text())
		}
		if scanner.Err() != nil && scanner.Err() != io.EOF {
			log.Printf("ERROR: Error reading server output: %v", scanner.Err())
		}
	}()

	// 启动一个 goroutine 等待进程结束并清理
	go func(cmd *exec.Cmd) {
		log.Printf("Minecraft server process (PID: %d) has started...", cmd.Process.Pid)
		cmd.Wait()
		log.Println("Minecraft server process has stopped.")
		mu.Lock()
		mcServerCmd = nil
		mu.Unlock()
	}(cmd)

	mcServerCmd = cmd
	w.Write([]byte("Minecraft server start command sent successfully."))
}

// the function to handle the shutdown of server
func handleStop(w http.ResponseWriter, r *http.Request) {
	if !checkStarted() {
		w.Write([]byte("The server can only be closed if it is already closed >_<"))
		return
	}
	conn, err := rcon.Dial(webConfig.RCON_HOST, webConfig.RCON_PASSWORD)
	if err != nil {
		http.Error(w, fmt.Sprintf("fialed to connect to RCON server:%v", err), http.StatusInternalServerError)
		log.Printf("RCON error while connecting:%v", err)
		return
	}
	defer conn.Close()

	response, err := conn.Execute("stop")
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to send command:%v", err), http.StatusInternalServerError)
		log.Printf("RCON fialed to send:%v", err)
		return
	}

	fmt.Fprintf(w, "command 'stop' is sent\n with response \n%s", response)
}

func (h *FileManager) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	files := h.ListFiles()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
	log.Println("web checked list")

}

func (h *FileManager) handleEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missising file id", http.StatusBadRequest)
		return
	}
	handler, ok := h.GetHandler(id)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	content, err := handler.ReadRaw()
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":      id,
		"name":    handler.Name,
		"content": content,
		"type":    handler.Type,
		"preview": handler.Preview,
		"path":    handler.Path,
	})
	log.Printf("file %s viewed", handler.Path)

}

func (h *FileManager) handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	handler, ok := h.GetHandler(req.ID)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	if handler.Preview {
		http.Error(w, "File is preview-only", http.StatusForbidden)
		return
	}

	if checkStarted() {
		http.Error(w, "Cannot edit files while server is running", http.StatusConflict)
		return
	}

	if err := handler.SaveRaw(req.Content); err != nil {
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		log.Printf("unable to save %s: %s", handler.Path, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	if handler.Path == configFilePosition {
		loadConfig(configFilePosition)
		log.Println("config file edited and reloaded successfully")
	}
	log.Printf("file %s edited and saved", handler.Path)
}

// the function to write the log to the web
func handlelog(w http.ResponseWriter, r *http.Request) {
	if !webConfig.SHOW_LOG {
		_, err := w.Write([]byte("server is running now"))
		if err != nil {
			log.Fatalln("Failed to write", err)
		}
	} else {
		path := os.ExpandEnv(webConfig.SERVER_PATH)
		lines, err := goTail(path+"/logs/latest.log", 50)

		if err != nil {
			log.Println("filed to read", webConfig.SERVER_PATH, err)
			http.Error(w, "unable to read", http.StatusInternalServerError)
			return
		}
		data := []byte(strings.Join(lines, "\n"))
		_, err = w.Write(data)

		if err != nil {
			log.Println("Failed to write", err)
		}
	}
}

// handleCommand to handle single command from web
func handleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to requedt body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	command := string(body)
	if command == "" {
		http.Error(w, "Command is empty", http.StatusBadRequest)
		return
	}

	log.Printf("Received command from web: %s", command)

	if !checkStarted() {
		log.Println("the server is not started yet")
		if command == "start" {
			resp, err := http.Get("http://127.0.0.1:" + webConfig.PORT + "/api/start")
			if err != nil {
				log.Println("Error closing server", err)
			} else {
				resp.Body.Close()
			}
			return
		}
		w.Write([]byte("It is pointless to send commands when server is down >_< (type \"start\" to start the server)"))
		return
	}

	conn, err := rcon.Dial(webConfig.RCON_HOST, webConfig.RCON_PASSWORD)
	if err != nil {
		log.Printf("Unable to connect to server: %e", err)
		http.Error(w, "RCON connection failed", http.StatusInternalServerError)
		return
	}

	defer conn.Close()

	resp, err := conn.Execute(command)
	if err != nil {
		log.Printf("failed to send the order: %e", err)
		http.Error(w, fmt.Sprintf("Command failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Println(resp)
	w.Write([]byte(resp))
}

// handleCommands handles multiple commands (newline-separated) in one request
func handleCommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	raw := strings.TrimSpace(string(body))
	if raw == "" {
		http.Error(w, "Commands are empty", http.StatusBadRequest)
		return
	}

	commands := strings.FieldsFunc(raw, func(c rune) bool { return c == '\n' })
	if len(commands) == 0 {
		http.Error(w, "No commands found", http.StatusBadRequest)
		return
	}

	log.Printf("Received %d commands from web", len(commands))

	if !checkStarted() {
		w.Write([]byte("It is pointless to send commands when server is down >_< (type \"start\" to start the server)"))
		return
	}

	conn, err := rcon.Dial(webConfig.RCON_HOST, webConfig.RCON_PASSWORD)
	if err != nil {
		log.Printf("Unable to connect to server: %e", err)
		http.Error(w, "RCON connection failed", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	var responses []string
	for _, cmd := range commands {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		resp, err := conn.Execute(cmd)
		if err != nil {
			log.Printf("failed to send command %q: %e", cmd, err)
			responses = append(responses, fmt.Sprintf("> %s\n[error] %v", cmd, err))
		} else {
			log.Printf("command %q executed", cmd)
			responses = append(responses, fmt.Sprintf("> %s\n%s", cmd, resp))
		}
	}

	w.Write([]byte(strings.Join(responses, "\n---\n")))
}

func saveConfig(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}

	return os.WriteFile("config.json", data, 0644)
}

func getDefaultConfig() Config {
	return Config{
		RCON_HOST:     "0.0.0.0:25575",
		RCON_PASSWORD: "1234abcd",
		PORT:          "8080",
		START_COMMAND: "java -Xmx6G -jar ./server.jar nogui",
		SERVER_PATH:   "$HOME/server/",
		SHOW_LOG:      true,
		FILES: []FileConfig{
			{
				PATH:    "$HOME/server/server.properties",
				NAME:    "server.properties",
				PREVIEW: true,
			},
		},
	}
}

func init() {
	log.Println("Initializing...")
	editConfigFiles = NewFileManager()
	configFilePosition = "config.json"
	for i, arg := range os.Args {
		if arg == "-c" || arg == "--config" {
			if i+1 < len(os.Args) {
				configFilePosition = os.Args[i+1]
				break
			}
		} else if arg == "-d" || arg == "--debug" {
			DEBUG = true
		} else if arg == "-h" || arg == "--help" {
			fmt.Println("usage: web_for_server [option] ... <config.json>")
			fmt.Println("        -c / --config for setting config")
			fmt.Println("        -d / --debug  for start command line input")
			fmt.Println("        -h / --help   for help")
			os.Exit(0)
		}
	}
	log.Printf("using config file: %s\n", configFilePosition)
	loadConfig(configFilePosition)
}

func CheckExist(name string) (bool, error) {
	_, err := os.Stat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		} else {
			return false, fmt.Errorf("failed to get file status: %s", name)
		}
	}
	return true, nil
}

func loadConfig(file string) {
	b, err := CheckExist(file)
	fmt.Println("checking " + file + " file")
	if err != nil {
		log.Fatalf("Error occoured when checking config.json file, make sure that is exists: %v", err)
		os.Exit(1)
	}
	if !b {
		log.Println("config.json does not exit, creating...")
		webConfig = getDefaultConfig()
		err := saveConfig(webConfig)
		if err != nil {
			log.Println("failed to save config.json, please download it from the repo")
		}
	} else {
		fileContent, err := os.ReadFile(file)
		if err != nil {
			log.Fatalf("Error occoured when reading: %v", err)
			os.Exit(1)
		}
		if err := json.Unmarshal(fileContent, &webConfig); err != nil {
			log.Fatalf("Error unamarshalling JSON: %v", err)
		}
		log.Println("Config loaded successfully")
		log.Println("====================================")
		log.Printf("rcon_host: %s\n", webConfig.RCON_HOST)
		log.Printf("rcon_password: %s\n", webConfig.RCON_PASSWORD)
		log.Printf("port: %s\n", webConfig.PORT)
		log.Printf("server_path: %s\n", webConfig.SERVER_PATH)
		log.Printf("start_command: %s\n", webConfig.START_COMMAND)
		log.Printf("show_log: %t\n", webConfig.SHOW_LOG)
		log.Printf("debug: %t\n", DEBUG)
		log.Println("====================================")
	}
	for _, f := range webConfig.FILES {
		editFile := f.newFile()
		editConfigFiles.RegisterHandler(editFile.ID, *editFile)
	}
	log.Println("edit config files loaded successfully")
}

func main() {
	if DEBUG {
		startCli()
	}
	dist, err := fs.Sub(indexDir, "static")
	if err != nil {
		panic(err)
	}
	http.Handle("/", http.FileServer(http.FS(dist)))
	http.HandleFunc("/api/start", handleStart)
	http.HandleFunc("/api/stop", handleStop)
	http.HandleFunc("/api/checkstart", handlecheckStart)
	http.HandleFunc("/api/command", handleCommand)
	http.HandleFunc("/api/commands", handleCommands)
	http.HandleFunc("/api/file", editConfigFiles.handleList)
	http.HandleFunc("/api/file/edit", editConfigFiles.handleEdit)
	http.HandleFunc("/api/file/save", editConfigFiles.handleSave)
	http.HandleFunc("/api/log", handlelog)
	log.Println("Starting server on :" + webConfig.PORT)
	log.Fatal(http.ListenAndServe(":"+webConfig.PORT, nil))
}

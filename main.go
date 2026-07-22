package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
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
	DEBUG              bool
	editConfigFiles    *FileManager

	//go:embed static/*
	staticFS embed.FS
	//go:embed templates/*
	templateFS embed.FS

	templates *template.Template
)

func checkStarted() bool {
	return mcServerCmd != nil
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

// --- CLI ---

func startCli() {
	go func() {
		fmt.Println("Type 'exit' to quit the application.")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			command := strings.TrimSpace(scanner.Text())
			switch command {
			case "exit":
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
			case "start":
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
			case "reload":
				loadConfig(configFilePosition)
			case "help":
				fmt.Println("\"start\" to start the minecraft server")
				fmt.Println("\"stop\" to stop the minecraft server")
				fmt.Println("\"exit\" to exit the program")
				fmt.Println("\"reload\" to reload config")
				fmt.Println("\"help\" to get help for the program")
				fmt.Println("all other cases are send directly to the minecraft server as command")
			default:
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
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "Error reading from stdin: %v\n", err)
			os.Exit(1)
		}
	}()
}

// --- Template helpers ---

type StatusData struct {
	Class string
	Text  string
}

type LogData struct {
	Content template.HTML
}

type ListData struct {
	Files []File
}

type EditData struct {
	ID       string
	Name     string
	Path     string
	Content  string
	ReadOnly bool
}

func htmlerr(msg string) template.HTML {
	return template.HTML(`<div class="error-msg">` + template.HTMLEscapeString(msg) + `</div>`)
}

func htmlerrf(format string, a ...any) template.HTML {
	return htmlerr(fmt.Sprintf(format, a...))
}

func htmlok(msg string) template.HTML {
	return template.HTML(`<div>` + template.HTMLEscapeString(msg) + `</div>`)
}

func renderTemplate(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// --- Handlers ---

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	renderTemplate(w, "index.html", nil)
}

func handleEditPage(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing file id", http.StatusBadRequest)
		return
	}
	handler, ok := editConfigFiles.GetFile(id)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	content, err := handler.ReadRaw()
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}
	renderTemplate(w, "edit.html", EditData{
		ID:       id,
		Name:     handler.Name,
		Path:     handler.Path,
		Content:  content,
		ReadOnly: handler.Preview || checkStarted(),
	})
	log.Printf("file %s is edited", handler.Name)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	running := checkStarted()
	if running {
		renderTemplate(w, "_status.html", StatusData{Class: "status-running", Text: "running"})
	} else {
		renderTemplate(w, "_status.html", StatusData{Class: "status-stopped", Text: "stopped"})
	}
}

func handleStart(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	if checkStarted() {
		renderTemplate(w, "_log.html", LogData{Content: htmlerr("serer is already running")})
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
		renderTemplate(w, "_log.html", LogData{Content: htmlerrf("failed to start: %v", err)})
		return
	}

	go func() {
		defer stdout.Close()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if scanner.Err() == nil {
				fmt.Printf("%s\n", scanner.Text())
			} else {
				log.Fatalln("failed to get pipe")
			}
		}
	}()

	go func(cmd *exec.Cmd) {
		log.Printf("Minecraft server process (PID: %d) has started...", cmd.Process.Pid)
		cmd.Wait()
		log.Println("Minecraft server process has stopped.")
		mu.Lock()
		mcServerCmd = nil
		mu.Unlock()
	}(cmd)

	mcServerCmd = cmd
	renderTemplate(w, "_log.html", LogData{Content: htmlok("server have started")})
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	if !checkStarted() {
		renderTemplate(w, "_log.html", LogData{Content: htmlok("server not running")})
		return
	}
	conn, err := rcon.Dial(webConfig.RCON_HOST, webConfig.RCON_PASSWORD)
	if err != nil {
		log.Printf("RCON error while connecting:%v", err)
		renderTemplate(w, "_log.html", LogData{Content: htmlerrf("RCON connection failed: %v", err)})
		return
	}
	defer conn.Close()

	response, err := conn.Execute("stop")
	if err != nil {
		log.Printf("RCON failed to send:%v", err)
		renderTemplate(w, "_log.html", LogData{Content: htmlerrf("failed to send stop command: %v", err)})
		return
	}
	renderTemplate(w, "_log.html", LogData{Content: htmlok("stop command sent\n" + response)})
}

func handleLog(w http.ResponseWriter, r *http.Request) {
	if !checkStarted() {
		renderTemplate(w, "_log.html", LogData{Content: template.HTML(`<div class="log-placeholder">server not running</div>`)})
		return
	}
	if !webConfig.SHOW_LOG {
		renderTemplate(w, "_log.html", LogData{Content: htmlok("server started")})
		return
	}

	path := os.ExpandEnv(webConfig.SERVER_PATH)
	lines, err := goTail(path+"/logs/latest.log", 50)
	if err != nil {
		log.Println("failed to read", path, err)
		renderTemplate(w, "_log.html", LogData{Content: htmlerr("unable to read log")})
		return
	}

	escaped := template.HTMLEscapeString(strings.Join(lines, "\n"))
	renderTemplate(w, "_log.html", LogData{Content: template.HTML(escaped)})
}

func handleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var command string
	if r.Header.Get("Content-Type") == "application/json" {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()
		command = strings.TrimSpace(string(body))
	} else {
		command = strings.TrimSpace(r.FormValue("command"))
	}

	if command == "" {
		renderTemplate(w, "_log.html", LogData{Content: htmlerr("command cannot be empty")})
		return
	}

	log.Printf("Received command from web: %s", command)

	if !checkStarted() {
		renderTemplate(w, "_log.html", LogData{Content: htmlerr("server down, unable to send command")})
		return
	}

	conn, err := rcon.Dial(webConfig.RCON_HOST, webConfig.RCON_PASSWORD)
	if err != nil {
		log.Printf("Unable to connect to server: %v", err)
		renderTemplate(w, "_log.html", LogData{Content: htmlerrf("RCON connection failed: %v", err)})
		return
	}
	defer conn.Close()

	resp, err := conn.Execute(command)
	if err != nil {
		log.Printf("failed to send the order: %v", err)
		renderTemplate(w, "_log.html", LogData{Content: htmlerrf("command failed to execute: %v", err)})
		return
	}
	log.Println(resp)
	renderTemplate(w, "_log.html", LogData{Content: template.HTML(template.HTMLEscapeString(resp))})
}

func handleCommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var raw string
	if r.Header.Get("Content-Type") == "application/json" {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()
		raw = strings.TrimSpace(string(body))
	} else {
		raw = strings.TrimSpace(r.FormValue("command"))
	}

	if raw == "" {
		renderTemplate(w, "_log.html", LogData{Content: htmlerr("command cannot be empty")})
		return
	}

	commands := strings.FieldsFunc(raw, func(c rune) bool { return c == '\n' })
	log.Printf("Received %d commands from web", len(commands))

	if !checkStarted() {
		renderTemplate(w, "_log.html", LogData{Content: htmlerr("server down")})
		return
	}

	conn, err := rcon.Dial(webConfig.RCON_HOST, webConfig.RCON_PASSWORD)
	if err != nil {
		log.Printf("Unable to connect to server: %v", err)
		renderTemplate(w, "_log.html", LogData{Content: htmlerrf("RCON connection failed: %v", err)})
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
			log.Printf("failed to send command %q: %v", cmd, err)
			responses = append(responses, fmt.Sprintf("&gt; %s\n[error] %v", template.HTMLEscapeString(cmd), err))
		} else {
			log.Printf("command %q executed", cmd)
			responses = append(responses, fmt.Sprintf("&gt; %s\n%s", template.HTMLEscapeString(cmd), template.HTMLEscapeString(resp)))
		}
	}

	renderTemplate(w, "_log.html", LogData{Content: template.HTML(strings.Join(responses, "\n---\n"))})
}

func (h *FileManager) handleList(w http.ResponseWriter, r *http.Request) {
	files := h.ListFiles()
	renderTemplate(w, "_file_list.html", ListData{Files: files})
}

func (h *FileManager) handleEdit(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing file id", http.StatusBadRequest)
		return
	}
	handler, ok := h.GetFile(id)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	content, err := handler.ReadRaw()
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}
	renderTemplate(w, "_file_edit.html", EditData{
		ID:       id,
		Name:     handler.Name,
		Path:     handler.Path,
		Content:  content,
		ReadOnly: handler.Preview || checkStarted(),
	})
	log.Printf("file %s viewed", handler.Path)
}

func (h *FileManager) handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var reqID, reqContent string
	if r.Header.Get("Content-Type") == "application/json" {
		var req struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		reqID = req.ID
		reqContent = req.Content
	} else {
		reqID = r.FormValue("id")
		reqContent = r.FormValue("content")
	}

	handler, ok := h.GetFile(reqID)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	if handler.Preview {
		w.Write([]byte(`<span class="error-msg">file is read only</span>`))
		return
	}

	if checkStarted() {
		w.Write([]byte(`<span class="error-msg">server is running, unable to edit</span>`))
		return
	}

	if err := handler.SaveRaw(reqContent); err != nil {
		log.Printf("unable to save %s: %s", handler.Path, err)
		w.Write([]byte(`<span class="error-msg">failed to save</span>`))
		return
	}

	if handler.Path == configFilePosition {
		loadConfig(configFilePosition)
		log.Println("config file edited and reloaded successfully")
	}
	w.Write([]byte(`<span class="success-msg">✅ saved successfully</span>`))
	log.Printf("file %s saved", handler.Path)
}

// --- Config ---

func saveConfig(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFilePosition, data, 0644)
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
		if arg == "-g" || arg == "--generate" {
			if i+1 < len(os.Args) {
				configFilePosition = os.Args[i+1]
				if err := saveConfig(getDefaultConfig()); err != nil {
					log.Printf("failed to save config at %s: %v", configFilePosition, err)
					os.Exit(1)
				}
			}
			log.Printf("config file is generated successfully at %s", configFilePosition)
			os.Exit(0)
		} else if arg == "-c" || arg == "--config" {
			if i+1 < len(os.Args) {
				configFilePosition = os.Args[i+1]
			}
		} else if arg == "-d" || arg == "--debug" {
			DEBUG = true
		} else if arg == "-h" || arg == "--help" {
			fmt.Println("usage: web_for_server [option] ... <path to your config>")
			fmt.Println("        -g / --generate to generate default configuration at path you want")
			fmt.Println("        -c / --config for setting config file path you are using")
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
	var err error
	templates, err = template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		log.Fatalf("failed to parse templates: %v", err)
	}

	if DEBUG {
		startCli()
	}

	dist, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(dist))))
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/edit", handleEditPage)
	http.HandleFunc("/api/status", handleStatus)
	http.HandleFunc("/api/start", handleStart)
	http.HandleFunc("/api/stop", handleStop)
	http.HandleFunc("/api/command", handleCommand)
	http.HandleFunc("/api/commands", handleCommands)
	http.HandleFunc("/api/file", editConfigFiles.handleList)
	http.HandleFunc("/api/file/save", editConfigFiles.handleSave)
	http.HandleFunc("/api/log", handleLog)

	log.Println("Starting server on :" + webConfig.PORT)
	log.Fatal(http.ListenAndServe(":"+webConfig.PORT, nil))
}

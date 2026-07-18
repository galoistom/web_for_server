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
	"strings"
	"sync"

	"github.com/gorcon/rcon"
)

type Config struct {
	RCON_HOST     string `json:"rcon_host"`
	RCON_PASSWORD string `json:"rcon_password"`
	PORT          string `json:"port"`
	SERVER_PATH   string `json:"server_path"`
	SHOW_LOG      string `json:"show_log"`
	START_COMMAND string `json:"start_command"`
}

var (
	mcServerCmd *exec.Cmd  // 存储服务器进程的命令对象
	mu          sync.Mutex // 保护共享变量的锁
	webConfig   Config
	//go:embed static/*
	indexDir embed.FS
	DEBUG    bool
)

// to check whether the serer started
func checkStarted() bool {
	if mcServerCmd != nil {
		return true
	}
	return false
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
					fmt.Println("Exiting application...")
					if checkStarted() {
						resp, err := http.Get("http://127.0.0.1:" + webConfig.PORT + "/api/stop")
						if err != nil {
							fmt.Println("unable to connect to server", err)
						} else {
							resp.Body.Close()
						}
					}
					os.Exit(0)
				} // 安全退出程序
			case "start":
				{
					resp, err := http.Get("http://127.0.0.1:" + webConfig.PORT + "/api/start")
					if err != nil {
						fmt.Println("Error start server", err)
					} else {
						resp.Body.Close()
					}
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
		fmt.Println("failed to start server precess: ", err)
		w.WriteHeader(http.StatusInternalServerError)
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
		_, err := w.Write([]byte("The server can only be closed if it is already closed >_<"))
		if err != nil {
			fmt.Println("failed to write", err)
		}
	}
	conn, err := rcon.Dial(webConfig.RCON_HOST, webConfig.RCON_PASSWORD)
	if err != nil {
		http.Error(w, fmt.Sprintf("fialed to connect to RCON server:%v", err), http.StatusInternalServerError)
		log.Printf("RCON error while connecting:%v", err)
		return
	}
	defer conn.Close()

	// 发送 "stop" 命令
	response, err := conn.Execute("stop")
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to send command:%v", err), http.StatusInternalServerError)
		log.Printf("RCON fialed to send:%v", err)
		return
	}

	fmt.Fprintf(w, "command 'stop' is sent\n with response \n%s", response)
}

func handlenolog(w http.ResponseWriter, r *http.Request) {
	_, err := w.Write([]byte("server is running now"))
	if err != nil {
		log.Fatalln("Failed to write", err)
	}
}

// the function to write the log to the web
func handlelog(w http.ResponseWriter, r *http.Request) {
	path := os.ExpandEnv(webConfig.SERVER_PATH)
	cmd := exec.Command("tail", "-n", "50", "logs/latest.log")
	cmd.Dir = path
	data, err := cmd.Output()
	if err != nil {
		fmt.Println("filed to read", webConfig.SERVER_PATH, err)
		http.Error(w, "unable to read", http.StatusInternalServerError)
		return
	}

	_, err = w.Write(data)
	if err != nil {
		fmt.Println("Failed to write", err)
	}
}

// handleCommand to handle command from web
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

	// 从 URL 查询参数中获取 "command"
	command := string(body)
	if command == "" {
		http.Error(w, "Command is empty", http.StatusBadRequest)
		return
	}

	log.Printf("Received command from web: %s", command)

	if !checkStarted() {
		fmt.Println("the server is not started yet")
		if command == "start" {
			resp, err := http.Get("http://127.0.0.1:" + webConfig.PORT + "/api/start")
			if err != nil {
				fmt.Println("Error closing server", err)
			} else {
				resp.Body.Close()
			}
		}
		w.Write([]byte("It is pointless to send commands when server is down >_< (type \"start\" to start the server)"))
		return
	}

	conn, err := rcon.Dial(webConfig.RCON_HOST, webConfig.RCON_PASSWORD)
	if err != nil {
		fmt.Println("Unable to connect to server", err)
		return
	}

	defer conn.Close()

	resp, err := conn.Execute(command)
	if err != nil {
		fmt.Println("failed to send the order", err)
	} else {
		fmt.Println(resp)
		w.Write([]byte(resp))
	}

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
		SHOW_LOG:      "true",
	}
}

func init() {
	log.Println("Initializing...")
	file := "config.json"
	for i, arg := range os.Args {
		if arg == "-c" || arg == "--config" {
			if i+1 < len(os.Args) {
				file = os.Args[i+1]
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
	log.Printf("using config file: %s\n", file)
	loadConfig(file)
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
		//reading config file
		fileContent, err := os.ReadFile(file)
		if err != nil {
			log.Fatalf("Error occoured when reading: %v", err)
			os.Exit(1)
		}
		err = json.Unmarshal(fileContent, &webConfig)
		if err != nil {
			log.Fatalf("Error unamarshalling JSON: %v", err)
		} else {
			log.Println("Config loaded successfully")
		}
	}

}

func main() {
	log.Println("====================================")
	log.Printf("rcon_host: %s\n", webConfig.RCON_HOST)
	log.Printf("rcon_password: %s\n", webConfig.RCON_PASSWORD)
	log.Printf("port: %s\n", webConfig.PORT)
	log.Printf("server_path: %s\n", webConfig.SERVER_PATH)
	log.Printf("start_command: %s\n", webConfig.START_COMMAND)
	log.Printf("show_log: %s\n", webConfig.SHOW_LOG)
	log.Printf("mod: %t\n", DEBUG)
	log.Println("====================================")
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
	if webConfig.SHOW_LOG == "true" {
		http.HandleFunc("/api/log", handlelog)
	} else {
		http.HandleFunc("/api/log", handlenolog)
	}

	log.Println("Starting server on :" + webConfig.PORT)
	log.Fatal(http.ListenAndServe(":"+webConfig.PORT, nil))
}

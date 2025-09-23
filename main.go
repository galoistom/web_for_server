package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/gorcon/rcon"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type Config struct {
	RCON_HOST     string
	RCON_PASSWORD string
	PORT          string
	LOG_POST      string
}

var (
	mcServerCmd *exec.Cmd  // 存储服务器进程的命令对象
	mu          sync.Mutex // 保护共享变量的锁
	webConfig   Config
)

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
						fmt.Println("Error closing server", err)
					} else {
						resp.Body.Close()
					}
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
	}()
}

func handlecheckStart(w http.ResponseWriter, r *http.Request) {
	if checkStarted() {
		w.Write([]byte("running"))
	} else {
		w.Write([]byte("stopped"))
	}
	// fmt.Println("checked")
}

// handleStart 函数，处理启动服务器的请求
func handleStart(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	if checkStarted() {
		http.Error(w, "Minecraft server is already running.", http.StatusConflict)
		return
	}

	// 启动 Minecraft 服务器进程

	cmd := exec.Command("/bin/bash", "./server.sh")

	// 启动进程
	err := cmd.Start()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start server process: %v", err), http.StatusInternalServerError)
		return
	}
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
		http.Error(w, fmt.Sprintf("无法连接到 RCON 服务器：%v", err), http.StatusInternalServerError)
		log.Printf("RCON 连接错误：%v", err)
		return
	}
	defer conn.Close()

	// 发送 "stop" 命令
	response, err := conn.Execute("stop")
	if err != nil {
		http.Error(w, fmt.Sprintf("发送命令失败：%v", err), http.StatusInternalServerError)
		log.Printf("RCON 命令错误：%v", err)
		return
	}

	fmt.Fprintf(w, "命令 'stop' 已发送。\n服务器响应：\n%s", response)
}

// the function to write the log to the web
func handlelog(w http.ResponseWriter, r *http.Request) {

	data, err := os.ReadFile(webConfig.LOG_POST)
	if err != nil {
		fmt.Println("filed to read", err)
		http.Error(w, "unable to read", http.StatusInternalServerError)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		fmt.Println("Failed to write", err)
	}
}

// handleCommand 处理来自网页端的命令请求
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

func init() {
	log.Println("Initializing...")

	fileContent, err := os.ReadFile("config.json")
	if err != nil {
		log.Fatalf("Error occoured when reading: %v", err)
		return
	}

	err = json.Unmarshal(fileContent, &webConfig)
	if err != nil {
		log.Fatalf("Error unamarshalling JSON: %v", err)
	} else {
		log.Println("Config loaded successfully")
	}
}

func main() {
	startCli()
	http.Handle("/", http.FileServer(http.Dir("./static")))

	http.HandleFunc("/api/start", handleStart)
	http.HandleFunc("/api/stop", handleStop)
	http.HandleFunc("/api/log", handlelog)
	http.HandleFunc("/api/checkstart", handlecheckStart)
	http.HandleFunc("/api/command", handleCommand)

	log.Println("Starting server on :" + webConfig.PORT)
	log.Fatal(http.ListenAndServe(":"+webConfig.PORT, nil))
}

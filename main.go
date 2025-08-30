package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	//"time"

	"github.com/gorcon/rcon"
)

var (
	mcServerCmd *exec.Cmd  // 存储服务器进程的命令对象
	mu          sync.Mutex // 保护共享变量的锁
)

// RCON配置
const (
	RCON_HOST     = "0.0.0.0:25575"
	RCON_PASSWORD = "1234abcd" // 替换为你的 RCON 密码
)

func checkStarted() bool {
	if mcServerCmd != nil {
		return true
	}
	return false
}

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
						resp, err := http.Get("http://127.0.0.1:8080/api/stop")
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
					resp, err := http.Get("http://127.0.0.1:8080/api/start")
					if err != nil {
						fmt.Println("Error closing server", err)
					} else {
						resp.Body.Close()
					}
				}
			default:
				{
					conn, err := rcon.Dial(RCON_HOST, RCON_PASSWORD)
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

// handleStop 函数，处理停止服务器的请求
func handleStop(w http.ResponseWriter, r *http.Request) {
	conn, err := rcon.Dial(RCON_HOST, RCON_PASSWORD)
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

func handleWrite(w http.ResponseWriter, r *http.Request) {
	filename := "/home/liuziming/server/logs/latest.log"
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Println("filed to read")
		http.Error(w, "unable to read", http.StatusInternalServerError)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		fmt.Println("Failed to write")
	}
}

func main() {
	startCli()
	http.Handle("/", http.FileServer(http.Dir("./static")))

	http.HandleFunc("/api/start", handleStart)
	http.HandleFunc("/api/stop", handleStop)
	http.HandleFunc("/api/log", handleWrite)
	http.HandleFunc("/api/checkstart", handlecheckStart)

	log.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

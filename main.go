package main

import (
	"bufio"
	"embed"
	"fmt"
	"github.com/gorcon/rcon"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type Config struct {
	TCP_HOST      string       `json:"tcp_host"`
	RCON_HOST     string       `json:"rcon_host"`
	RCON_PASSWORD string       `json:"rcon_password"`
	PORT          string       `json:"port"`
	SERVER_PATH   string       `json:"server_path"`
	SHOW_LOG      bool         `json:"show_log"`
	START_COMMAND string       `json:"start_command"`
	FILES         []FileConfig `json:"files"`
}

var (
	mcServerCmd        *exec.Cmd
	latestLog          io.ReadCloser
	mu                 sync.Mutex
	webConfig          Config
	configFilePosition string
	DEBUG              bool
	editConfigFiles    *FileManager
	tcpPassword        string = "1234abcd"

	//go:embed static/*
	staticFS embed.FS
	//go:embed templates/*
	templateFS embed.FS

	templates *template.Template
)

func checkStarted() bool {
	return mcServerCmd != nil
}

func readerOutput(stdout io.ReadCloser, output func(string), stop <-chan bool) error {
	defer stdout.Close()
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case <-stop:
			return nil
		default:
			output(scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func startServer() (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if checkStarted() {
		return "", fmt.Errorf("server running")
	}

	cmd := exec.Command("sh", "-c", webConfig.START_COMMAND)
	cmd.Dir = os.ExpandEnv(webConfig.SERVER_PATH)

	var err error
	latestLog, err = cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("failed to get out pipe:%v", err)
	}
	err = cmd.Start()
	if err != nil {
		log.Printf("failed to start server process: %v", err)
		return "", fmt.Errorf("failed to start: %v", err)
	}

	go func(cmd *exec.Cmd) {
		log.Printf("Minecraft server process (PID: %d) has started...", cmd.Process.Pid)
		cmd.Wait()
		log.Println("Minecraft server process has stopped.")
		mu.Lock()
		mcServerCmd = nil
		latestLog = nil
		mu.Unlock()
	}(cmd)
	mcServerCmd = cmd
	return "server have started", nil
}

func Commands(command string) (string, error) {
	if command == "" {
		return "", fmt.Errorf("command cannot be empty")
	}
	log.Printf("Received command from web: %s", command)

	if !checkStarted() {
		return "", fmt.Errorf("server down, unable to send command")
	}

	conn, err := rcon.Dial(webConfig.RCON_HOST, webConfig.RCON_PASSWORD)
	if err != nil {
		log.Printf("Unable to connect to server: %v", err)
		return "", fmt.Errorf("RCON connection failed: %v", err)
	}
	defer conn.Close()
	resp, err := conn.Execute(command)
	if err != nil {
		log.Printf("failed to send the order: %v", err)
		return "", fmt.Errorf("command failed to execute: %v", err)
	}
	log.Println(resp)
	return resp, nil
}

// --- CLI ---

func startCli() {
	go func() {
		fmt.Println("Type 'exit' to quit the application.")
		scanner := bufio.NewScanner(os.Stdin)
		stop := make(chan bool)
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
				result, err := startServer()
				go readerOutput(latestLog, func(a string) {
					fmt.Printf("%s\n", a)
				}, stop)
				if err != nil {
					log.Fatalf("failed to start: %v", err)
				}
				log.Println(result)
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
				resp, err := Commands(command)
				if err != nil {
					fmt.Println("fialed to send the order:", err)
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

func tcpConnect() {
	ip := webConfig.TCP_HOST
	log.Printf("tcp listening: %s", ip)
	listner, err := net.Listen("tcp", ip)
	if err != nil {
		log.Printf("failed to listen: %v", err)
		return
	}
	for true {
		conn, err := listner.Accept()
		if err != nil {
			log.Printf("failed to accept: %v", err)
			continue
		}
		log.Printf("connected from %s", conn.RemoteAddr())
		go handleConnect(conn)
	}
}

func handleConnect(c net.Conn) {
	defer c.Close()
	fmt.Fprint(c, "Password:")
	buf := make([]byte, 1024)
	n, err := c.Read(buf)
	if err != nil {
		log.Printf("failed to read: %v", err)
	}
	if strings.TrimSpace(string(buf[:n])) != tcpPassword {
		fmt.Fprintln(c, "password incorrect")
		return
	}
	fmt.Fprintln(c, "welcome!")
	connected := false
	stop := make(chan bool)
	for true {
		buf := make([]byte, 1024)
		n, err := c.Read(buf)
		if err != nil {
			log.Printf("failed to read: %v", err)
		}
		if strings.TrimSpace(string(buf[:n])) == "exit" || n == 0 {
			log.Printf("tcp connection %s stopped\n", c.RemoteAddr())
			break
		}
		message := strings.TrimRight(string(buf[:n]), "\r\n")
		log.Printf("received from %s : %s", c.RemoteAddr(), message)
		switch {
		case message == "status":
			if checkStarted() {
				fmt.Fprintln(c, "server running...")
			} else {
				fmt.Fprintln(c, "server down...")
			}
		case message == "start":
			if checkStarted() {
				fmt.Fprintln(c, "Server is already started...")
				continue
			}
			result, err := startServer()
			go readerOutput(latestLog, func(a string) {
				fmt.Fprintf(c, "%s\n", a)
			}, stop)
			if err != nil {
				fmt.Fprintf(c, "failed to start: %v\n", err)
			}
			connected = true
			fmt.Fprintln(c, result)
		case message == "connect":
			if !checkStarted() {
				fmt.Fprintln(c, "server not started yet...")
			} else if connected {
				fmt.Fprintln(c, "already connected")
			} else {
				go readerOutput(latestLog, func(a string) {
					fmt.Fprintf(c, "%s\n", a)
				}, stop)
			}
		case message == "disconnect":
			if !connected {
				fmt.Fprintln(c, "not connected yet")
			} else {
				stop <- true
			}
		case message == "reload":
			loadConfig(configFilePosition)
			fmt.Fprintf(c, "config reloaded: %+v", webConfig)
		case message == "help":
			fmt.Fprintln(c, "\"help\"                to get help for the program")
			fmt.Fprintln(c, "\"exit\"                to exit the connection")
			fmt.Fprintln(c, "\"start\"               to start the minecraft server")
			fmt.Fprintln(c, "\"stop\"                to stop the minecraft server")
			fmt.Fprintln(c, "\"reload\"              to reload config")
			fmt.Fprintln(c, "\"status\"              to check server status")
			fmt.Fprintln(c, "\"log <number>\"        to see log")
			fmt.Fprintln(c, "\"command: <commands>\" to send command to the minecraft server")
		case strings.HasPrefix(message, "log "):
			num, err := strconv.Atoi(message[4:])
			if err != nil {
				fmt.Fprintln(c, "invalid input format, \n usage: log <numbers of line you want to see>")
				continue
			}
			path := os.ExpandEnv(webConfig.SERVER_PATH)
			lines, err := goTail(path+"/logs/latest.log", num)
			if err != nil {
				fmt.Fprintf(c, "failed to read log: %v\n", err)
			} else {
				fmt.Fprintln(c, strings.Join(lines, "\n"))
			}
		case strings.HasPrefix(message, "command ") || message == "stop":
			if !checkStarted() {
				fmt.Println("the server is not started yet")
				continue
			}
			var command string
			if message == "stop" {
				command = "stop"
			} else {
				command = message[8:]
			}
			resp, err := Commands(command)
			if err != nil {
				fmt.Fprintln(c, "failed to send the order", err)
			} else {
				fmt.Fprintln(c, resp)
			}
		default:
			fmt.Fprintln(c, "invalid command")
		}
	}
}

// --- Config ---

func init() {
	log.Println("Initializing...")
	editConfigFiles = NewFileManager()
	configFilePosition = "config.json"
	for i, arg := range os.Args {
		switch arg {
		case "-g", "--generagte":
			if i+1 < len(os.Args) {
				configFilePosition = os.Args[i+1]
				if err := saveConfig(getDefaultConfig()); err != nil {
					log.Printf("failed to save config at %s: %v", configFilePosition, err)
					os.Exit(1)
				}
			}
			log.Printf("config file is generated successfully at %s", configFilePosition)
			os.Exit(0)
		case "-c", "--config":
			if i+1 < len(os.Args) {
				configFilePosition = os.Args[i+1]
			}
		case "-d", "--debug":
			DEBUG = true
		case "-h", "--help":
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

func main() {
	var err error
	templates, err = template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		log.Fatalf("failed to parse templates: %v", err)
	}

	go tcpConnect()
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
	http.HandleFunc("/download/log", handleDownloadLog)

	log.Println("Starting server on :" + webConfig.PORT)
	log.Fatal(http.ListenAndServe(":"+webConfig.PORT, nil))
}

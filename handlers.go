package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
)

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
	result, err := startServer()
	if err != nil {
		renderTemplate(w, "_log.html", LogData{Content: htmlerr(result)})
	}
	renderTemplate(w, "_log.html", LogData{Content: htmlok(result)})
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	if !checkStarted() {
		renderTemplate(w, "_log.html", LogData{Content: htmlok("server not running")})
		return
	}
	resp, err := Commands("stop")
	if err != nil {
		log.Printf("RCON failed to send:%v", err)
		renderTemplate(w, "_log.html", LogData{Content: htmlerrf("failed to send stop command: %v", err)})
		return
	}
	renderTemplate(w, "_log.html", LogData{Content: htmlok("stop command sent\n" + resp)})
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

	path := webConfig.SERVER_PATH
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
	resp, err := Commands(command)
	if err != nil {
		renderTemplate(w, "_log.html", LogData{Content: htmlerr(err.Error())})
	} else {
		renderTemplate(w, "_log.html",
			LogData{Content: template.HTML(template.HTMLEscapeString(resp))})
	}
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

	var responses []string
	for _, cmd := range commands {
		cmd = strings.TrimSpace(cmd)
		resp, err := Commands(cmd)
		if err != nil {
			log.Printf("command %q with error: %v", cmd, err)
			responses = append(responses, fmt.Sprintf("&gt; %s\n[error] %v", template.HTMLEscapeString(cmd), err))
		} else {
			log.Printf("command %q executed", cmd)
			responses = append(responses, fmt.Sprintf("&gt; %s\n%s", template.HTMLEscapeString(cmd), template.HTMLEscapeString(resp)))
		}
	}

	renderTemplate(w, "_log.html", LogData{Content: template.HTML(strings.Join(responses, "\n---\n"))})
}

func handleDownloadLog(w http.ResponseWriter, r *http.Request) {
	filePath := filepath.Join(webConfig.SERVER_PATH, "logs", "latest.log")
	w.Header().Set("Content-Disposition", "attachment; filename=latest.log")
	w.Header().Set("Content-Type", "text/plain")
	http.ServeFile(w, r, filePath)
	log.Println("request for download:", filePath)
}

func (h *FileManager) handleList(w http.ResponseWriter, r *http.Request) {
	files := h.ListFiles()
	renderTemplate(w, "_file_list.html", ListData{Files: files})
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

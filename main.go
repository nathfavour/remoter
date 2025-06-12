package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/nathfavour/remoter/ffmpeg"
	"github.com/nathfavour/remoter/vnc"
)

type Config struct {
	VNC       bool   `json:"vnc"`
	FFmpeg    bool   `json:"ffmpeg"`
	Display   string `json:"display"`
	Res       string `json:"res"`
	Port      int    `json:"port"`
	Framerate int    `json:"framerate"`
	WebDir    string `json:"webdir"` // New field for React project directory
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	clients    = make(map[*websocket.Conn]bool)
	clientsMux sync.RWMutex
)

func defaultConfig() *Config {
	return &Config{
		VNC:       false,
		FFmpeg:    true,
		Display:   ":0.0",
		Res:       "1920x1080x24",
		Port:      8081,
		Framerate: 25,
		WebDir:    "web", // Default React project directory
	}
}

func getConfigPath() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}
	return filepath.Join(usr.HomeDir, ".remoter.json"), nil
}

func loadOrCreateConfig() (*Config, error) {
	path, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := defaultConfig()
			if err := saveConfig(cfg, path); err != nil {
				return nil, fmt.Errorf("failed to create default config: %w", err)
			}
			log.Printf("Created default configuration at %s", path)
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	updated := false
	if cfg.Port == 0 {
		cfg.Port = 8081
		updated = true
	}
	if cfg.Framerate == 0 {
		cfg.Framerate = 25
		updated = true
	}
	if cfg.WebDir == "" {
		cfg.WebDir = "web"
		updated = true
	}

	if updated {
		if err := saveConfig(&cfg, path); err != nil {
			log.Printf("Warning: failed to update config file: %v", err)
		}
	}

	return &cfg, nil
}

func saveConfig(cfg *Config, path string) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

func broadcast(data []byte) {
	clientsMux.RLock()
	defer clientsMux.RUnlock()

	var disconnected []*websocket.Conn
	for client := range clients {
		if err := client.WriteMessage(websocket.BinaryMessage, data); err != nil {
			disconnected = append(disconnected, client)
		}
	}

	if len(disconnected) > 0 {
		clientsMux.RUnlock()
		clientsMux.Lock()
		for _, client := range disconnected {
			client.Close()
			delete(clients, client)
		}
		clientsMux.Unlock()
		clientsMux.RLock()
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	clientsMux.Lock()
	clients[conn] = true
	totalClients := len(clients)
	clientsMux.Unlock()

	log.Printf("New WebSocket client connected. Total clients: %d", totalClients)

	conn.SetCloseHandler(func(code int, text string) error {
		clientsMux.Lock()
		delete(clients, conn)
		totalClients := len(clients)
		clientsMux.Unlock()
		log.Printf("Client disconnected. Total clients: %d", totalClients)
		return nil
	})

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			clientsMux.Lock()
			delete(clients, conn)
			totalClients := len(clients)
			clientsMux.Unlock()
			log.Printf("Client disconnected due to read error: %v. Total clients: %d", err, totalClients)
			break
		}
	}
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" && r.Method != "PUT" {
		http.Error(w, "Only POST/PUT methods allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Printf("FFmpeg stream connected")
	defer log.Printf("FFmpeg stream disconnected")

	buf := make([]byte, 4096)
	totalBytes := 0
	frameCount := 0

	for {
		n, err := r.Body.Read(buf)
		if n > 0 {
			totalBytes += n
			broadcast(buf[:n])
			frameCount++

			if frameCount%100 == 0 {
				clientsMux.RLock()
				clientCount := len(clients)
				clientsMux.RUnlock()
				log.Printf("Streamed %d bytes, %d frames to %d clients", totalBytes, frameCount, clientCount)
			}
		}
		if err != nil {
			log.Printf("Stream ended after %d bytes, %d frames", totalBytes, frameCount)
			break
		}
	}
}

func buildReactApp(webDir string) error {
	absWebDir, err := filepath.Abs(filepath.Join(filepath.Dir(os.Args[0]), webDir))
	if err != nil {
		return fmt.Errorf("failed to resolve webdir: %w", err)
	}
	cmd := exec.Command("pnpm", "build")
	cmd.Dir = absWebDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Printf("Building React app with 'pnpm build' in %s...", absWebDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to build React app: %w", err)
	}
	return nil
}

func startScreenShareServer(port int, webDir string) error {
	if err := buildReactApp(webDir); err != nil {
		return err
	}

	absWebDir, err := filepath.Abs(filepath.Join(filepath.Dir(os.Args[0]), webDir))
	if err != nil {
		return fmt.Errorf("failed to resolve webdir: %w", err)
	}
	buildDir := filepath.Join(absWebDir, "build")
	fs := http.FileServer(http.Dir(buildDir))
	http.Handle("/", fs)

	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/stream", handleStream)

	addr := fmt.Sprintf("0.0.0.0:%d", port)
	log.Printf("Starting screen share server on %s", addr)

	go func() {
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	return nil
}

func startServices(cfg *Config) error {
	servicesStarted := 0

	if cfg.FFmpeg {
		if err := startScreenShareServer(cfg.Port, cfg.WebDir); err != nil {
			return fmt.Errorf("failed to start screen share server: %w", err)
		}

		go func() {
			log.Printf("Starting FFmpeg service...")
			if err := ffmpeg.StartFFmpeg(cfg.Display, cfg.Res, cfg.Port); err != nil {
				log.Fatalf("FFmpeg error: %v", err)
			}
		}()
		servicesStarted++
		log.Printf("FFmpeg service configured")
	}

	if cfg.VNC {
		go func() {
			log.Printf("Starting VNC service...")
			if err := vnc.StartVNC(cfg.Display, cfg.Res); err != nil {
				log.Fatalf("VNC error: %v", err)
			}
		}()
		servicesStarted++
		log.Printf("VNC service configured")
	}

	if servicesStarted == 0 {
		return fmt.Errorf("no services enabled in configuration")
	}

	log.Printf("Started %d service(s)", servicesStarted)
	return nil
}

func main() {
	log.Printf("Starting Remoter v1.0")

	cfg, err := loadOrCreateConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded: Display=%s, Port=%d, VNC=%t, FFmpeg=%t",
		cfg.Display, cfg.Port, cfg.VNC, cfg.FFmpeg)

	if err := startServices(cfg); err != nil {
		log.Printf("No screen sharing services enabled.")
		log.Printf("Edit ~/.remoter.json to enable VNC and/or FFmpeg.")
		log.Printf("Example configuration:")
		example := defaultConfig()
		example.FFmpeg = true
		data, _ := json.MarshalIndent(example, "", "  ")
		log.Printf("\n%s", string(data))
		return
	}

	log.Printf("Remoter is running. Visit http://localhost:%d to view the stream.", cfg.Port)
	log.Printf("Press Ctrl+C to stop.")

	select {}
}

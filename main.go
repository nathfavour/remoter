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

// Config represents the application configuration
type Config struct {
	VNC       bool   `json:"vnc"`
	FFmpeg    bool   `json:"ffmpeg"`
	Display   string `json:"display"`
	Res       string `json:"res"`
	Port      int    `json:"port"`
	Framerate int    `json:"framerate"`
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for development
		},
	}
	clients    = make(map[*websocket.Conn]bool)
	clientsMux sync.RWMutex
)

// defaultConfig returns a default configuration
func defaultConfig() *Config {
	return &Config{
		VNC:       false,
		FFmpeg:    true,
		Display:   ":0.0",
		Res:       "1920x1080x24",
		Port:      8081,
		Framerate: 25,
	}
}

// getConfigPath returns the path to the configuration file
func getConfigPath() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}
	return filepath.Join(usr.HomeDir, ".remoter.json"), nil
}

// loadOrCreateConfig loads configuration from file or creates default if not exists
func loadOrCreateConfig() (*Config, error) {
	path, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	// Try to open existing config file
	f, err := os.Open(path)
	if err != nil {
		// Create default config if file doesn't exist
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

	// Parse existing config
	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Ensure required fields have default values
	updated := false
	if cfg.Port == 0 {
		cfg.Port = 8081
		updated = true
	}
	if cfg.Framerate == 0 {
		cfg.Framerate = 25
		updated = true
	}

	// Save updated config if changes were made
	if updated {
		if err := saveConfig(&cfg, path); err != nil {
			log.Printf("Warning: failed to update config file: %v", err)
		}
	}

	return &cfg, nil
}

// saveConfig saves configuration to file
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

// broadcast sends data to all connected WebSocket clients
func broadcast(data []byte) {
	clientsMux.RLock()
	defer clientsMux.RUnlock()

	var disconnected []*websocket.Conn
	for client := range clients {
		if err := client.WriteMessage(websocket.BinaryMessage, data); err != nil {
			disconnected = append(disconnected, client)
		}
	}

	// Remove disconnected clients
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

// handleWebSocket handles WebSocket connections
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

	// Handle client disconnect
	conn.SetCloseHandler(func(code int, text string) error {
		clientsMux.Lock()
		delete(clients, conn)
		totalClients := len(clients)
		clientsMux.Unlock()
		log.Printf("Client disconnected. Total clients: %d", totalClients)
		return nil
	})

	// Keep connection alive by reading messages (and discarding them)
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

// handleStream handles FFmpeg stream data
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

// startScreenShareServer starts the HTTP server for screen sharing
func startScreenShareServer(port int) error {
	// Serve React build directory as static files
	buildDir := "/home/nathfavour/Documents/code/nathfavour/remoter/web/build"

	// Check if build directory exists
	if _, err := os.Stat(buildDir); os.IsNotExist(err) {
		log.Printf("Warning: React build directory not found at %s", buildDir)
		log.Printf("Please run 'npm run build' in the web directory first")
		// Serve a simple message instead
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `
				<html>
					<head><title>Remoter</title></head>
					<body>
						<h1>Remoter Screen Share</h1>
						<p>React build not found. Please run 'npm run build' in the web directory.</p>
					</body>
				</html>
			`)
		})
	} else {
		fs := http.FileServer(http.Dir(buildDir))
		http.Handle("/", fs)
	}

	// WebSocket endpoint
	http.HandleFunc("/ws", handleWebSocket)

	// Stream endpoint for FFmpeg
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

// startReactDevServer starts the React development server (optional)
func startReactDevServer() error {
	webDir := "/home/nathfavour/Documents/code/nathfavour/remoter/web"

	// Check if web directory exists
	if _, err := os.Stat(webDir); os.IsNotExist(err) {
		log.Printf("Warning: React web directory not found at %s", webDir)
		return fmt.Errorf("web directory not found")
	}

	log.Printf("Starting React development server...")
	cmd := exec.Command("npm", "start")
	cmd.Dir = webDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	go func() {
		if err := cmd.Run(); err != nil {
			log.Printf("React dev server exited: %v", err)
		}
	}()

	return nil
}

// startServices starts the configured services (VNC and/or FFmpeg)
func startServices(cfg *Config) error {
	servicesStarted := 0

	if cfg.FFmpeg {
		if err := startScreenShareServer(cfg.Port); err != nil {
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

	// Load configuration
	cfg, err := loadOrCreateConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded: Display=%s, Port=%d, VNC=%t, FFmpeg=%t",
		cfg.Display, cfg.Port, cfg.VNC, cfg.FFmpeg)

	// Start configured services
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

	// Keep the application running
	select {}
}

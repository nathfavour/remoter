package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/nathfavour/remoter/ffmpeg"
	"github.com/nathfavour/remoter/vnc"
)

type Config struct {
	VNC     bool   `json:"vnc"`
	FFmpeg  bool   `json:"ffmpeg"`
	Display string `json:"display"` // Set to ":0.0", ":1", etc. in ~/.remoter.json
	Res     string `json:"res"`
	Port    int    `json:"port"`
}

func defaultConfig() *Config {
	return &Config{
		VNC:     false,
		FFmpeg:  true,
		Display: ":0.0", // Use real display instead of virtual
		Res:     "1920x1080x24",
		Port:    8081,
	}
}

func configPath() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(usr.HomeDir, ".remoter.json"), nil
}

func loadOrCreateConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		cfg := defaultConfig()
		b, _ := json.MarshalIndent(cfg, "", "  ")
		_ = os.WriteFile(path, b, 0644)
		return cfg, nil
	}
	defer f.Close()
	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	if cfg.Port == 0 {
		cfg.Port = 8081
		b, _ := json.MarshalIndent(cfg, "", "  ")
		_ = os.WriteFile(path, b, 0644)
	}
	return &cfg, nil
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for development
		},
	}
	clients    = make(map[*websocket.Conn]bool)
	clientsMux sync.Mutex
)

func broadcast(data []byte) {
	clientsMux.Lock()
	defer clientsMux.Unlock()
	for c := range clients {
		err := c.WriteMessage(websocket.BinaryMessage, data)
		if err != nil {
			c.Close()
			delete(clients, c)
		}
	}
}

func startScreenShareServer(port int) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `
<!DOCTYPE html>
<html>
<head>
  <title>Remoter Screen Share</title>
  <style>
    html, body { margin: 0; padding: 0; height: 100%; background: #000; }
    #video-canvas { display: block; margin: 0 auto; background: #000; max-width: 100%; max-height: 100%; }
    #status { color: white; position: absolute; top: 10px; left: 10px; }
  </style>
</head>
<body>
  <div id="status">Connecting...</div>
  <canvas id="video-canvas"></canvas>
  <script src="https://cdn.jsdelivr.net/npm/jsmpeg@0.2.1/jsmpeg.min.js"></script>
  <script>
    var canvas = document.getElementById('video-canvas');
    var status = document.getElementById('status');
    var url = "ws://" + location.host + "/ws";
    
    console.log("Connecting to:", url);
    status.textContent = "Connecting to " + url;
    
    var player = new JSMpeg.Player(url, {
      canvas: canvas, 
      autoplay: true, 
      audio: false,
      onVideoDecode: function(decoder, time) {
        status.textContent = "Live - " + decoder.width + "x" + decoder.height;
      }
    });
    
    // Check WebSocket connection
    setTimeout(function() {
      if (player.source && player.source.socket) {
        if (player.source.socket.readyState === WebSocket.OPEN) {
          status.textContent = "Connected, waiting for video...";
        } else {
          status.textContent = "WebSocket connection failed";
        }
      }
    }, 2000);
  </script>
</body>
</html>
		`)
	})

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}
		clientsMux.Lock()
		clients[conn] = true
		clientsMux.Unlock()
		log.Printf("New WebSocket client connected. Total clients: %d", len(clients))

		// Handle client disconnect
		conn.SetCloseHandler(func(code int, text string) error {
			clientsMux.Lock()
			delete(clients, conn)
			clientsMux.Unlock()
			log.Printf("Client disconnected. Total clients: %d", len(clients))
			return nil
		})
	})

	http.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" && r.Method != "PUT" {
			http.Error(w, "Only POST/PUT allowed", http.StatusMethodNotAllowed)
			return
		}
		log.Printf("FFmpeg stream connected")
		buf := make([]byte, 4096)
		totalBytes := 0
		for {
			n, err := r.Body.Read(buf)
			if n > 0 {
				totalBytes += n
				broadcast(buf[:n])
				if totalBytes%10000 == 0 { // Log every ~10KB
					log.Printf("Streamed %d bytes to %d clients", totalBytes, len(clients))
				}
			}
			if err != nil {
				log.Printf("Stream ended after %d bytes", totalBytes)
				break
			}
		}
		r.Body.Close()
	})

	addr := fmt.Sprintf("0.0.0.0:%d", port)
	go func() {
		log.Printf("Remoter screen share server listening on %s", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()
}

func main() {
	cfg, err := loadOrCreateConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	started := false

	if cfg.FFmpeg {
		startScreenShareServer(cfg.Port)
		go func() {
			if err := ffmpeg.StartFFmpeg(cfg.Display, cfg.Res, cfg.Port); err != nil {
				log.Fatalf("FFmpeg error: %v", err)
			}
		}()
		started = true
	}

	if cfg.VNC {
		go func() {
			if err := vnc.StartVNC(cfg.Display, cfg.Res); err != nil {
				log.Fatalf("VNC error: %v", err)
			}
		}()
		started = true
	}

	if !started {
		fmt.Println("No screen sharing service enabled in config. Edit ~/.remoter.json to enable VNC and/or FFmpeg.")
		return
	}

	select {} // Keep running
}

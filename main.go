package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/nathfavour/remoter/ffmpeg"
	"github.com/nathfavour/remoter/vnc"
)

type Config struct {
	Mode    string `json:"mode"`    // "vnc" or "ffmpeg"
	Display string `json:"display"` // e.g. ":1"
	Res     string `json:"res"`     // e.g. "1920x1080x24"
}

func getPrimaryIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "unknown"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			return ip.String()
		}
	}
	return "unknown"
}

func startIPBroadcastServer(ip string, port string) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "<h1>Device IP: %s</h1>", ip)
	})
	go func() {
		addr := "0.0.0.0:" + port
		log.Printf("Broadcasting IP at http://%s/", addr)
		err := http.ListenAndServe(addr, nil)
		if err != nil {
			log.Fatalf("Failed to start IP broadcast server on port %s: %v\nTry running with sudo or choose a higher port (e.g., 8080, 4246).", port, err)
		}
	}()
}

func loadConfig() (*Config, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(usr.HomeDir, ".remoter.json")
	f, err := os.Open(configPath)
	if err != nil {
		// Create default config if not exists
		defaultCfg := &Config{
			Mode:    "vnc",
			Display: ":1",
			Res:     "1920x1080x24",
		}
		b, _ := json.MarshalIndent(defaultCfg, "", "  ")
		_ = os.WriteFile(configPath, b, 0644)
		return defaultCfg, nil
	}
	defer f.Close()
	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func main() {
	ip := getPrimaryIP()
	port := "8642"

	fmt.Printf("Device IP: %s\n", ip)
	startIPBroadcastServer(ip, port)

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	switch cfg.Mode {
	case "vnc":
		if err := vnc.StartVNC(cfg.Display, cfg.Res); err != nil {
			log.Fatalf("VNC error: %v", err)
		}
	case "ffmpeg":
		if err := ffmpeg.StartFFmpeg(cfg.Display, cfg.Res); err != nil {
			log.Fatalf("FFmpeg error: %v", err)
		}
	default:
		log.Fatalf("Unknown mode in config: %s", cfg.Mode)
	}

	select {} // Keep running
}
	// Start panel (tint2) for taskbar and app launcher
	cmd3 := exec.Command("tint2")
	cmd3.Env = append(os.Environ(), "DISPLAY="+display)
	if err := cmd3.Start(); err != nil {
		fmt.Printf("Warning: Failed to start panel: %v\n", err)
	}

	// Start a terminal with the wrapper script that sets DISPLAY permanently
	cmd4 := exec.Command(xtermPath)
	cmd4.Env = append(os.Environ(), "DISPLAY="+display)
	if err := cmd4.Start(); err != nil {
		fmt.Printf("Warning: Failed to start terminal: %v\n", err)
	}

	return nil
}

func main() {
	ip := getPrimaryIP()
	port := "8642"
	display := ":1"
	res := "1920x1080x24"

	fmt.Printf("Device IP: %s\n", ip)
	startIPBroadcastServer(ip, port)

	// Ensure dependencies
	for _, pkg := range []string{"x11vnc", "xvfb", "openbox", "pcmanfm", "xterm", "tint2"} {
		if err := ensureInstalled(pkg); err != nil {
			log.Fatalf("Failed to install %s: %v", pkg, err)
		}
	}

	// Start Xvfb
	if err := startXvfb(display, res); err != nil {
		log.Fatalf("Failed to start Xvfb: %v", err)
	}
	time.Sleep(2 * time.Second) // Give Xvfb time to initialize

	// Start desktop environment
	if err := startDesktop(display); err != nil {
		log.Fatalf("Failed to start desktop: %v", err)
	}
	time.Sleep(2 * time.Second) // Give desktop time to initialize

	// Start x11vnc
	if err := startX11vnc(display); err != nil {
		log.Fatalf("Failed to start x11vnc: %v", err)
	}

	select {} // Keep running
}

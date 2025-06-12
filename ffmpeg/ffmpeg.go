package ffmpeg

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
)

type Config struct {
	VNC     bool   `json:"vnc"`
	FFmpeg  bool   `json:"ffmpeg"`
	Display string `json:"display"`
	Res     string `json:"res"`
}

func getScreenInfo(display string) (string, string, error) {
	cmd := exec.Command("xdpyinfo", "-display", display)
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to run xdpyinfo: %w", err)
	}
	output := string(out)

	// Parse resolution (e.g., dimensions:    1920x1080 pixels)
	reRes := regexp.MustCompile(`dimensions:\s+(\d+)x(\d+) pixels`)
	mRes := reRes.FindStringSubmatch(output)
	if len(mRes) < 3 {
		return "", "", fmt.Errorf("could not parse screen resolution")
	}
	width := mRes[1]
	height := mRes[2]

	// Parse depth (e.g., depth of root window:    24 planes)
	reDepth := regexp.MustCompile(`depth of root window:\s+(\d+)`)
	mDepth := reDepth.FindStringSubmatch(output)
	depth := "24"
	if len(mDepth) >= 2 {
		depth = mDepth[1]
	}

	res := fmt.Sprintf("%sx%s", width, height)
	return res, depth, nil
}

func configPath() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(usr.HomeDir, ".remoter.json"), nil
}

func loadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveConfig(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(path, b, 0644)
}

func StartFFmpeg(display, res string, port int) error {
	// For real display, try :0.0 first, then fall back to config
	if display == ":0.0" {
		// Check if we can access the real display
		cmd := exec.Command("xdpyinfo", "-display", ":0.0")
		if err := cmd.Run(); err != nil {
			fmt.Printf("Cannot access display :0.0, trying :0...\n")
			display = ":0"
		}
	}

	// Get actual screen info
	actualRes, depth, err := getScreenInfo(display)
	if err != nil {
		fmt.Printf("Warning: %v. Using config values.\n", err)
		// Parse resolution from config
		if strings.Contains(res, "x") {
			parts := strings.Split(res, "x")
			if len(parts) >= 2 {
				actualRes = fmt.Sprintf("%sx%s", parts[0], parts[1])
			}
		} else {
			actualRes = "1366x768" // fallback
		}
		depth = "24"
	}

	// Update config if needed
	cfg, err := loadConfig()
	if err == nil {
		updated := false
		if cfg.Res != fmt.Sprintf("%sx%s", strings.Split(actualRes, "x")[0], strings.Split(actualRes, "x")[1])+"x"+depth {
			cfg.Res = fmt.Sprintf("%sx%sx%s", strings.Split(actualRes, "x")[0], strings.Split(actualRes, "x")[1], depth)
			updated = true
		}
		if cfg.Display != display {
			cfg.Display = display
			updated = true
		}
		if updated {
			_ = saveConfig(cfg)
		}
	}

	// The display argument is already configurable via config and passed to FFmpeg.

	// Compose ffmpeg command with supported framerate for MPEG1
	url := fmt.Sprintf("http://localhost:%d/stream", port)
	ffmpegArgs := []string{
		"-video_size", actualRes,
		"-framerate", "25", // <-- Use 25 instead of 15
		"-f", "x11grab",
		"-i", display,
		"-vcodec", "mpeg1video",
		"-b:v", "800k",
		"-f", "mpeg1video",
		url,
	}
	fmt.Printf("Starting FFmpeg: ffmpeg %s\n", strings.Join(ffmpegArgs, " "))

	cmd := exec.Command("ffmpeg", ffmpegArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

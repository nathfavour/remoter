package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"
)

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

func ensureInstalled(pkg string) error {
	cmd := exec.Command("which", pkg)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Installing %s...\n", pkg)
		install := exec.Command("sudo", "apt", "install", "-y", pkg)
		install.Stdout = os.Stdout
		install.Stderr = os.Stderr
		return install.Run()
	}
	return nil
}

func startXvfb(display, res string) error {
	cmd := exec.Command("pgrep", "-f", "Xvfb "+display)
	if err := cmd.Run(); err != nil {
		fmt.Println("Starting Xvfb...")
		return exec.Command("Xvfb", display, "-screen", "0", res).Start()
	}
	return nil
}

func startX11vnc(display string) error {
	fmt.Println("Starting x11vnc...")
	return exec.Command("x11vnc", "-display", display, "-forever").Start()
}

func startDesktop(display string) error {
	fmt.Println("Starting desktop environment...")

	// Create a profile script that sets DISPLAY permanently for the session
	profileScript := `export DISPLAY=` + display + `
export XAUTHORITY=/tmp/.X` + display[1:] + `-auth
`
	profilePath := "/tmp/vnc_profile"
	if err := os.WriteFile(profilePath, []byte(profileScript), 0644); err != nil {
		return err
	}

	// Create a wrapper script for xterm that sources the profile
	xtermScript := `#!/bin/bash
source /tmp/vnc_profile
exec xterm -e "bash --rcfile /tmp/vnc_profile"
`
	xtermPath := "/tmp/vnc_xterm.sh"
	if err := os.WriteFile(xtermPath, []byte(xtermScript), 0755); err != nil {
		return err
	}

	// Start window manager (openbox) with proper environment
	cmd1 := exec.Command("openbox")
	cmd1.Env = append(os.Environ(), "DISPLAY="+display)
	if err := cmd1.Start(); err != nil {
		return err
	}

	time.Sleep(1 * time.Second)

	// Start file manager
	cmd2 := exec.Command("pcmanfm", "--desktop")
	cmd2.Env = append(os.Environ(), "DISPLAY="+display)
	if err := cmd2.Start(); err != nil {
		fmt.Printf("Warning: Failed to start file manager: %v\n", err)
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

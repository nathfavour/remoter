package vnc

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"
)

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

	profileScript := `export DISPLAY=` + display + `
export XAUTHORITY=/tmp/.X` + display[1:] + `-auth
`
	profilePath := "/tmp/vnc_profile"
	if err := os.WriteFile(profilePath, []byte(profileScript), 0644); err != nil {
		return err
	}

	xtermScript := `#!/bin/bash
source /tmp/vnc_profile
exec xterm -e "bash --rcfile /tmp/vnc_profile"
`
	xtermPath := "/tmp/vnc_xterm.sh"
	if err := os.WriteFile(xtermPath, []byte(xtermScript), 0755); err != nil {
		return err
	}

	cmd1 := exec.Command("openbox")
	cmd1.Env = append(os.Environ(), "DISPLAY="+display)
	if err := cmd1.Start(); err != nil {
		return err
	}

	time.Sleep(1 * time.Second)

	cmd2 := exec.Command("pcmanfm", "--desktop")
	cmd2.Env = append(os.Environ(), "DISPLAY="+display)
	if err := cmd2.Start(); err != nil {
		fmt.Printf("Warning: Failed to start file manager: %v\n", err)
	}

	cmd3 := exec.Command("tint2")
	cmd3.Env = append(os.Environ(), "DISPLAY="+display)
	if err := cmd3.Start(); err != nil {
		fmt.Printf("Warning: Failed to start panel: %v\n", err)
	}

	cmd4 := exec.Command(xtermPath)
	cmd4.Env = append(os.Environ(), "DISPLAY="+display)
	if err := cmd4.Start(); err != nil {
		fmt.Printf("Warning: Failed to start terminal: %v\n", err)
	}

	return nil
}

func StartVNC(display, res string) error {
	for _, pkg := range []string{"x11vnc", "xvfb", "openbox", "pcmanfm", "xterm", "tint2"} {
		if err := ensureInstalled(pkg); err != nil {
			log.Fatalf("Failed to install %s: %v", pkg, err)
		}
	}

	if err := startXvfb(display, res); err != nil {
		return fmt.Errorf("Failed to start Xvfb: %w", err)
	}
	time.Sleep(2 * time.Second)

	if err := startDesktop(display); err != nil {
		return fmt.Errorf("Failed to start desktop: %w", err)
	}
	time.Sleep(2 * time.Second)

	if err := startX11vnc(display); err != nil {
		return fmt.Errorf("Failed to start x11vnc: %w", err)
	}

	return nil
}

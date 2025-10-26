package ui

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/eiannone/keyboard"
)

func BootSequence() {
	ClearScreen()
	frames := []string{
		"Initializing BlackBox.",
		"Initializing BlackBox..",
		"Initializing BlackBox...",
		"Loading core modules...",
		"Syncing node identity...",
		"Establishing secure channels...",
		"BlackBox Ready.",
	}

	for _, f := range frames {
		ClearScreen()
		fmt.Println(f)
		time.Sleep(400 * time.Millisecond)
	}

	ClearScreen()
}

func ClearScreen() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func ShowBanner() {
	out, err := exec.Command("bash", "assets/blackboxlogo.sh").Output()
	if err != nil {
		fmt.Println("BlackBox v0.1.0 — Developed by HackNoGood")
		return
	}
	fmt.Println(string(out))
}

func MainMenu() string {
	BootSequence() // 👈 plays the startup animation first
	options := []string{"Join existing host", "Host new lobby"}
	selected := 0

	if err := keyboard.Open(); err != nil {
		panic(err)
	}
	defer keyboard.Close()

	for {
		ClearScreen()
		ShowBanner()
		fmt.Println("──────────────────────────────────────────")
		fmt.Println("BlackBox v0.1.0 — Developed by HackNoGood")
		fmt.Println("──────────────────────────────────────────")

		for i, opt := range options {
			prefix := "  "
			if i == selected {
				prefix = "> " // highlight cursor
			}
			fmt.Printf("%s[%d] %s\n", prefix, i+1, opt)
		}

		fmt.Println("──────────────────────────────────────────")
		fmt.Println("Use ↑/↓ to navigate, Enter to select, or press 1/2 directly.")

		char, key, err := keyboard.GetKey()
		if err != nil {
			panic(err)
		}

		switch key {
		case keyboard.KeyArrowUp:
			if selected > 0 {
				selected--
			}
		case keyboard.KeyArrowDown:
			if selected < len(options)-1 {
				selected++
			}
		case keyboard.KeyEnter:
			return map[int]string{0: "join", 1: "host"}[selected]
		default:
			if char == '1' {
				return "join"
			} else if char == '2' {
				return "host"
			}
		}
	}
}

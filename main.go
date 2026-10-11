package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/psalwasser/trello-tui/internal/config"
	"github.com/psalwasser/trello-tui/internal/demo"
	"github.com/psalwasser/trello-tui/internal/trello"
	"github.com/psalwasser/trello-tui/internal/ui"
)

const usage = `trello-tui — a fast terminal interface for Trello

Usage:
  trello-tui            open the TUI (runs setup on first launch)
  trello-tui auth       (re)configure your Trello API key and token
  trello-tui --demo     try it with sample data, no account needed

Credentials are read from %s
or the TRELLO_API_KEY / TRELLO_TOKEN environment variables.
`

func main() {
	demoMode := flag.Bool("demo", false, "run with in-memory sample data")
	flag.Usage = func() { fmt.Fprintf(os.Stderr, usage, config.ConfigPath()) }
	flag.Parse()

	if flag.Arg(0) == "auth" {
		if _, err := runAuth(); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	var be trello.Backend
	if *demoMode {
		be = demo.New()
	} else {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error reading config:", err)
			os.Exit(1)
		}
		if cfg.APIKey == "" || cfg.Token == "" {
			if cfg, err = runAuth(); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
		}
		be = trello.NewClient(cfg.APIKey, cfg.Token)
	}

	p := tea.NewProgram(ui.New(be, *demoMode), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runAuth() (config.Config, error) {
	in := bufio.NewReader(os.Stdin)
	ask := func(q string) string {
		fmt.Print(q)
		s, _ := in.ReadString('\n')
		return strings.TrimSpace(s)
	}
	cfg, _ := config.Load()

	fmt.Println("Trello setup")
	fmt.Println()
	fmt.Println("1. Get an API key: open https://trello.com/power-ups/admin, create a")
	fmt.Println("   Power-Up (any name/workspace), then open its \"API key\" tab and generate one.")
	fmt.Println()
	key := ask(fmt.Sprintf("   API key%s: ", hintExisting(cfg.APIKey)))
	if key == "" {
		key = cfg.APIKey
	}
	if key == "" {
		return cfg, fmt.Errorf("an API key is required")
	}

	authURL := "https://trello.com/1/authorize?" + url.Values{
		"expiration":    {"never"},
		"scope":         {"read,write"},
		"response_type": {"token"},
		"name":          {"trello-tui"},
		"key":           {key},
	}.Encode()
	fmt.Println()
	fmt.Println("2. Authorize trello-tui to get a token. Opening:")
	fmt.Println("   " + authURL)
	openBrowser(authURL)
	fmt.Println()
	token := ask("   Token: ")
	if token == "" {
		return cfg, fmt.Errorf("a token is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	me, err := trello.NewClient(key, token).Me(ctx)
	if err != nil {
		return cfg, fmt.Errorf("verifying credentials: %w", err)
	}
	cfg = config.Config{APIKey: key, Token: token}
	if err := config.Save(cfg); err != nil {
		return cfg, err
	}
	fmt.Printf("\nSigned in as %s (@%s). Saved to %s\n", me.FullName, me.Username, config.ConfigPath())
	return cfg, nil
}

func openBrowser(u string) {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	_ = exec.Command(opener, u).Start()
}

func hintExisting(v string) string {
	if len(v) < 6 {
		return ""
	}
	return " [enter to keep …" + v[len(v)-4:] + "]"
}

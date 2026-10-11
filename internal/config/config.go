// Package config handles credentials, persistent UI state and the on-disk
// cache used to paint the UI instantly on startup.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	APIKey string `json:"api_key"`
	Token  string `json:"token"`
}

type State struct {
	LastBoardID string `json:"last_board_id,omitempty"`
}

func configDir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "trello-tui")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "trello-tui")
}

func cacheDir() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "trello-tui")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "trello-tui")
}

func ConfigPath() string { return filepath.Join(configDir(), "config.json") }
func statePath() string  { return filepath.Join(cacheDir(), "state.json") }

// Load reads the config file, with TRELLO_API_KEY / TRELLO_TOKEN taking
// precedence.
func Load() (Config, error) {
	var c Config
	data, err := os.ReadFile(ConfigPath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return c, err
	}
	if err == nil {
		if err := json.Unmarshal(data, &c); err != nil {
			return c, err
		}
	}
	if v := os.Getenv("TRELLO_API_KEY"); v != "" {
		c.APIKey = v
	}
	if v := os.Getenv("TRELLO_TOKEN"); v != "" {
		c.Token = v
	}
	return c, nil
}

func Save(c Config) error {
	return writeJSON(ConfigPath(), c, 0o600)
}

func LoadState() State {
	var s State
	_ = readJSON(statePath(), &s)
	return s
}

func SaveState(s State) error { return writeJSON(statePath(), s, 0o600) }

// ReadCache loads a cached JSON blob by name into v.
func ReadCache(name string, v any) error {
	return readJSON(filepath.Join(cacheDir(), name+".json"), v)
}

// WriteCache stores v as a cached JSON blob.
func WriteCache(name string, v any) error {
	return writeJSON(filepath.Join(cacheDir(), name+".json"), v, 0o600)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func writeJSON(path string, v any, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DownloadDir  string        `yaml:"download_dir"`
	Concurrency  int           `yaml:"concurrency"`
	RequestDelay time.Duration `yaml:"request_delay"`
	NoResume     bool          `yaml:"no_resume"`
	Sites        SitesConfig   `yaml:"sites"`
}

type SitesConfig struct {
	Gofile GofileConfig `yaml:"gofile"`
}

type GofileConfig struct {
	AccountToken string `yaml:"account_token"`
}

func defaults() Config {
	return Config{
		DownloadDir: defaultDownloadDir(),
	}
}

func Load() (*Config, error) {
	cfg := defaults()

	path, err := configPath()
	if err == nil {
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			_ = yaml.Unmarshal(data, &cfg)
		}
	}

	applyEnv(&cfg)
	cfg.DownloadDir = ExpandPath(cfg.DownloadDir)
	return &cfg, nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("MSD_DOWNLOAD_DIR"); v != "" {
		cfg.DownloadDir = v
	}
	if v := os.Getenv("MSD_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Concurrency = n
		}
	}
	if v := os.Getenv("MSD_GOFILE_TOKEN"); v != "" {
		cfg.Sites.Gofile.AccountToken = v
	} else if v := os.Getenv("GOFILE_TOKEN"); v != "" {
		cfg.Sites.Gofile.AccountToken = v
	}
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "msd", "config.yaml"), nil
}

func defaultDownloadDir() string {
	if dir := xdgUserDir("XDG_DOWNLOAD_DIR"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, "Downloads")
	}
	return "."
}

func xdgUserDir(key string) string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		configHome = filepath.Join(home, ".config")
	}

	data, err := os.ReadFile(filepath.Join(configHome, "user-dirs.dirs"))
	if err != nil {
		return ""
	}

	prefix := key + "="
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, prefix)), `"`)
		return ExpandPath(value)
	}
	return ""
}

func ExpandPath(path string) string {
	if path == "" {
		return path
	}

	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}

	return os.Expand(path, func(name string) string {
		if name == "XDG_DOWNLOAD_DIR" {
			return defaultDownloadDir()
		}
		return os.Getenv(name)
	})
}

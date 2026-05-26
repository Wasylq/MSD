package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DownloadDir  string        `yaml:"download_dir"`
	Concurrency  int           `yaml:"concurrency"`
	RequestDelay time.Duration `yaml:"request_delay"`
	NoResume     bool          `yaml:"no_resume"`
}

func defaults() Config {
	return Config{
		DownloadDir: ".",
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
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "msd", "config.yaml"), nil
}

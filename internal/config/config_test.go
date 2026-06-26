package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := defaults()
	want := filepath.Join(home, "Downloads")
	if cfg.DownloadDir != want {
		t.Errorf("default DownloadDir = %q, want %q", cfg.DownloadDir, want)
	}
	if cfg.Concurrency != 0 {
		t.Errorf("default Concurrency = %d, want 0", cfg.Concurrency)
	}
}

func TestLoad_NoConfigFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MSD_DOWNLOAD_DIR", "")
	t.Setenv("MSD_CONCURRENCY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(home, "Downloads")
	if cfg.DownloadDir != want {
		t.Errorf("DownloadDir = %q, want %q", cfg.DownloadDir, want)
	}
}

func TestLoad_DefaultDownloadDirUsesXDGUserDirs(t *testing.T) {
	home := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("MSD_DOWNLOAD_DIR", "")
	t.Setenv("MSD_CONCURRENCY", "")

	if err := os.WriteFile(filepath.Join(configHome, "user-dirs.dirs"), []byte(`XDG_DOWNLOAD_DIR="$HOME/Localized Downloads"`+"\n"), 0o644); err != nil {
		t.Fatalf("write user dirs: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(home, "Localized Downloads")
	if cfg.DownloadDir != want {
		t.Errorf("DownloadDir = %q, want %q", cfg.DownloadDir, want)
	}
}

func TestLoad_ConfigFile(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	configDir := filepath.Join(dir, "msd")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(`
download_dir: $HOME/downloads
concurrency: 8
sites:
  gofile:
    account_token: file-token
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("MSD_DOWNLOAD_DIR", "")
	t.Setenv("MSD_CONCURRENCY", "")
	t.Setenv("MSD_GOFILE_TOKEN", "")
	t.Setenv("GOFILE_TOKEN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wantDownloadDir := filepath.Join(home, "downloads")
	if cfg.DownloadDir != wantDownloadDir {
		t.Errorf("DownloadDir = %q, want %q", cfg.DownloadDir, wantDownloadDir)
	}
	if cfg.Concurrency != 8 {
		t.Errorf("Concurrency = %d, want 8", cfg.Concurrency)
	}
	if cfg.Sites.Gofile.AccountToken != "file-token" {
		t.Errorf("Gofile account token = %q, want file-token", cfg.Sites.Gofile.AccountToken)
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	configDir := filepath.Join(dir, "msd")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(`
download_dir: /tmp/from-file
concurrency: 4
sites:
  gofile:
    account_token: file-token
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("MSD_DOWNLOAD_DIR", "~/from-env")
	t.Setenv("MSD_CONCURRENCY", "12")
	t.Setenv("MSD_GOFILE_TOKEN", "env-token")
	t.Setenv("GOFILE_TOKEN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wantDownloadDir := filepath.Join(home, "from-env")
	if cfg.DownloadDir != wantDownloadDir {
		t.Errorf("DownloadDir = %q, want %q", cfg.DownloadDir, wantDownloadDir)
	}
	if cfg.Concurrency != 12 {
		t.Errorf("Concurrency = %d, want 12", cfg.Concurrency)
	}
	if cfg.Sites.Gofile.AccountToken != "env-token" {
		t.Errorf("Gofile account token = %q, want env-token", cfg.Sites.Gofile.AccountToken)
	}
}

func TestLoad_GofileTokenAlternateEnv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MSD_DOWNLOAD_DIR", "")
	t.Setenv("MSD_CONCURRENCY", "")
	t.Setenv("MSD_GOFILE_TOKEN", "")
	t.Setenv("GOFILE_TOKEN", "alternate-env-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Sites.Gofile.AccountToken != "alternate-env-token" {
		t.Errorf("Gofile account token = %q, want alternate-env-token", cfg.Sites.Gofile.AccountToken)
	}
}

func TestLoad_InvalidEnvConcurrency(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MSD_DOWNLOAD_DIR", "")
	t.Setenv("MSD_CONCURRENCY", "notanumber")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Concurrency != 0 {
		t.Errorf("Concurrency = %d, want 0 (invalid env ignored)", cfg.Concurrency)
	}
}

func TestLoad_ConfigCanUseXDGDownloadDirVariable(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	configDir := filepath.Join(dir, "msd")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "user-dirs.dirs"), []byte(`XDG_DOWNLOAD_DIR="$HOME/Localized Downloads"`+"\n"), 0o644); err != nil {
		t.Fatalf("write user dirs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(`
download_dir: ${XDG_DOWNLOAD_DIR}/msd
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("MSD_DOWNLOAD_DIR", "")
	t.Setenv("MSD_CONCURRENCY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(home, "Localized Downloads", "msd")
	if cfg.DownloadDir != want {
		t.Errorf("DownloadDir = %q, want %q", cfg.DownloadDir, want)
	}
}

func TestExpandPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := ExpandPath("~/nested")
	want := filepath.Join(home, "nested")
	if got != want {
		t.Errorf("ExpandPath = %q, want %q", got, want)
	}
}

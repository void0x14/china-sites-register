package kahin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RuntimeConfig, Kahin motorunun çalışma anı yapılandırmasıdır.
//
// Hiçbir yol sabit gömülmez; sırayla ortam değişkenleri ve bilinen
// kurulum konumları taranır (KAHIN_* değişkenleri docs/kahin-mcp-protokol.md §6.2).
type RuntimeConfig struct {
	Python  string // kahin paketini import edebilen python
	Dir     string // kahin paketinin kökü
	Lock    string // KAHIN_BROWSER_LOCK_PATH (boşsa varsayılan)
	Home    string // KAHIN_HOME (boşsa varsayılan)
	Profile string // KAHIN_PROFILE_DIR (opsiyonel)
}

// Env, alt sürece verilecek ortam değişkenlerini üretir.
func (c RuntimeConfig) Env() []string {
	var env []string
	if c.Lock != "" {
		env = append(env, "KAHIN_BROWSER_LOCK_PATH="+c.Lock)
	}
	if c.Home != "" {
		env = append(env, "KAHIN_HOME="+c.Home)
	}
	if c.Profile != "" {
		env = append(env, "KAHIN_PROFILE_DIR="+c.Profile)
	}
	return env
}

// Options, RuntimeConfig'i NewClient girdisine çevirir.
func (c RuntimeConfig) Options() Options {
	return Options{
		Python:        c.Python,
		Dir:           c.Dir,
		Env:           c.Env(),
		ClientName:    "china-sites-register",
		ClientVersion: "0.1.0",
	}
}

// Resolve, çalışma anında Kahin kurulumunu bulur.
//
// Sıra:
//  1. KAHIN_PYTHON + KAHIN_DIR ortam değişkenleri
//  2. Bilinen kurulum konumları (repo .venv, kurulu venv) — taranır
func Resolve() (RuntimeConfig, error) {
	cfg := RuntimeConfig{
		Python:  os.Getenv("KAHIN_PYTHON"),
		Dir:     os.Getenv("KAHIN_DIR"),
		Lock:    os.Getenv("KAHIN_BROWSER_LOCK_PATH"),
		Home:    os.Getenv("KAHIN_HOME"),
		Profile: os.Getenv("KAHIN_PROFILE_DIR"),
	}
	if cfg.Python != "" && cfg.Dir != "" {
		return cfg, nil
	}

	// 1) Proje yerleşimi: mcp-projelerim/cdp-kahin-mcp
	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, fmt.Errorf("kahin: ev dizini bulunamadı: %w", err)
	}
	candidates := []string{
		filepath.Join(home, "Documents", "mcp-projelerim", "cdp-kahin-mcp"),
		filepath.Join(home, "mcp-projelerim", "cdp-kahin-mcp"),
		filepath.Join(home, "cdp-kahin-mcp"),
	}
	for _, dir := range candidates {
		if cfg.Dir == "" {
			if _, err := os.Stat(filepath.Join(dir, "kahin", "oracle.py")); err != nil {
				continue
			}
			cfg.Dir = dir
		}
		if cfg.Python == "" {
			for _, rel := range []string{".venv/bin/python", "venv/bin/python"} {
				p := filepath.Join(dir, rel)
				if st, err := os.Stat(p); err == nil && !st.IsDir() {
					cfg.Python = p
					break
				}
			}
		}
		if cfg.Python != "" && cfg.Dir != "" {
			break
		}
	}

	// 2) Kurulu venv + kurulu paket: python -c "import kahin"
	if cfg.Python == "" {
		if p := findInstalledKahinPython(home); p != "" {
			cfg.Python = p
			if cfg.Dir == "" {
				cfg.Dir = home // import edilebilir olduğu için cwd serbest
			}
		}
	}

	if cfg.Dir == "" {
		cfg.Dir = home
	}
	if cfg.Python == "" {
		return cfg, fmt.Errorf("kahin: python yorumlayıcısı bulunamadı (KAHIN_PYTHON ayarlayın)")
	}
	// /tmp gibi yabancı .so içeren cwd'ler python import'unu bozar
	// (bkz. docs/kahin-mcp-protokol.md §6.5). Bu yüzden asla cwd=/tmp kullanılmaz.
	if strings.HasPrefix(cfg.Dir, "/tmp") {
		cfg.Dir = home
	}
	return cfg, nil
}

// findInstalledKahinPython, kahin paketini import edebilen bir python arar.
func findInstalledKahinPython(home string) string {
	cands := []string{
		filepath.Join(home, ".local", "share", "kahin", "venv", "bin", "python"),
	}
	for _, p := range cands {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// Toolset, bir kayıt akışı için gereken tool'ları doğrular.
type Toolset struct {
	Required []string
	Present  map[string]bool
	Missing  []string
}

// CheckTools, gerekli tool'ların varlığını doğrular.
func CheckTools(tools []ToolInfo) Toolset {
	required := []string{
		"kahin_browser_start", "kahin_browser_stop", "kahin_engine_health",
		"kahin_navigate", "kahin_mirage_snapshot", "kahin_mirage_click",
		"kahin_mirage_fill_form", "kahin_mirage_dom_action", "kahin_mirage_eval",
		"kahin_screenshot",
		// CAPTCHA sürükleme ve gerçek fare olayları için (canlı doğrulandı).
		"kahin_mirage_mouse_move", "kahin_mirage_mouse_down",
		"kahin_mirage_mouse_up", "kahin_mirage_mouse_click",
		"kahin_mirage_mouse_trajectory",
	}
	ts := Toolset{Required: required, Present: map[string]bool{}}
	for _, t := range tools {
		ts.Present[t.Name] = true
	}
	for _, r := range required {
		if !ts.Present[r] {
			ts.Missing = append(ts.Missing, r)
		}
	}
	return ts
}

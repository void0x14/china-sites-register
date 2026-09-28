// Package accounts, laptop'taki Gitee hesap havuzunu okur.
//
// Kaynak: ~/gitee/hesaplar.txt ve ~/gitee/patlar.txt (laptop, SSH ile).
// Yol bilgisi çalışma anında bulunur; kural dosyasına gömülmez.
//
// hesaplar.txt satır biçimi (gerçek dosyadan):
//
//	ACC: <email> | <email şifresi> | <gitee kullanıcı adı> | <gitee şifresi> | <profil url> | <tarih>
//
// Örnek:
//
//	ACC: git1061316704@uberip.com | 8Nsav9MVckutgC | git_5913 | f381acb421 | https://gitee.com/git_5913 | 2026-09-26
//
// GİRİŞTE KULLANILAN ŞİFRE 4. ALANDIR (gitee şifresi). 2. alan e-posta
// hesabının şifresidir, Gitee girişinde kullanılmaz.
package accounts

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Account, tek bir Gitee hesabıdır.
type Account struct {
	Email     string // 1. alan
	EmailPass string // 2. alan (e-posta hesabı şifresi)
	Username  string // 3. alan (gitee kullanıcı adı)
	Password  string // 4. alan (GITEE ŞİFRESİ — girişte bu kullanılır)
	Profile   string // 5. alan
	Created   string // 6. alan
	PAT       string // patlar.txt karşılığı (varsa)
	Raw       string
}

var reACC = regexp.MustCompile(`(?i)^ACC:\s*(.*)$`)

// ParseHesaplar, hesaplar.txt içeriğini hesap listesine çevirir.
func ParseHesaplar(content string) ([]Account, error) {
	var out []Account
	sc := bufio.NewScanner(strings.NewReader(content))
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		m := reACC.FindStringSubmatch(line)
		if m == nil {
			continue // ACC: ile başlamayan satır atlanır
		}
		body := strings.TrimSpace(m[1])
		parts := strings.Split(body, "|")
		if len(parts) < 4 {
			return nil, fmt.Errorf("accounts: satır %d en az 4 alan olmalı, %d var: %q", lineNo, len(parts), line)
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		a := Account{
			Email:     parts[0],
			EmailPass: parts[1],
			Username:  parts[2],
			Password:  parts[3],
			Raw:       line,
		}
		if len(parts) > 4 {
			a.Profile = parts[4]
		}
		if len(parts) > 5 {
			a.Created = parts[5]
		}
		if a.Email == "" || a.Username == "" || a.Password == "" {
			return nil, fmt.Errorf("accounts: satır %d eksik alan (email/username/şifre): %q", lineNo, line)
		}
		out = append(out, a)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("accounts: hiç hesap bulunamadı")
	}
	return out, nil
}

// ParsePATs, patlar.txt içeriğini token listesine çevirir.
func ParsePATs(content string) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		t := strings.TrimSpace(sc.Text())
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		out = append(out, t)
	}
	return out
}

// AttachPATs, PAT listesini kullanıcı adı eşleşmesiyle hesaplara bağlar.
//
// patlar.txt sıralıdır ve hesaplar.txt ile birebir eşleşmeyebilir; bu yüzden
// PAT'ler doğrudan Gitee API'sinden doğrulanıp login adına göre bağlanır
// (bkz. VerifyPATs). Burada yalnızca sıra eşleşmesi varsa bağlanır.
func AttachPATs(accs []Account, pats []string) {
	if len(pats) == 0 {
		return
	}
	if len(pats) == len(accs) {
		for i := range accs {
			accs[i].PAT = pats[i]
		}
		return
	}
	for i := range accs {
		if i < len(pats) {
			accs[i].PAT = pats[i]
		}
	}
}

// ByUsername, kullanıcı adına göre hesap bulur.
func ByUsername(accs []Account, username string) *Account {
	for i := range accs {
		if strings.EqualFold(accs[i].Username, username) {
			return &accs[i]
		}
	}
	return nil
}

// ByEmail, e-postaya göre hesap bulur.
func ByEmail(accs []Account, email string) *Account {
	for i := range accs {
		if strings.EqualFold(accs[i].Email, email) {
			return &accs[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Laptop erişimi (SSH)
// ---------------------------------------------------------------------------

// SSHSource, hesap dosyalarının laptop'taki yerini tarif eder.
type SSHSource struct {
	Host string // ör. void0x14@192.168.1.28
	Dir  string // ör. ~/gitee
	Key  string // opsiyonel özel anahtar yolu
}

// DefaultSSHSource, ortamdan/SSH yapılandırmasından hedefi çözer.
//
// Sabit IP gömülmez: SSH config veya SSH_LAPTOP_HOST ortam değişkeni kullanılır.
func DefaultSSHSource() SSHSource {
	return SSHSource{
		Host: envOr("SSH_LAPTOP_HOST", ""),
		Dir:  envOr("SSH_LAPTOP_GITEE_DIR", "~/gitee"),
		Key:  envOr("SSH_LAPTOP_KEY", ""),
	}
}

// Fetch, hedef dosyaları SSH ile okur.
func (s SSHSource) Fetch() (hesaplar string, patlar string, err error) {
	if strings.TrimSpace(s.Host) == "" {
		return "", "", fmt.Errorf("accounts: SSH hedefi yok (SSH_LAPTOP_HOST boş)")
	}
	dir := s.Dir
	if dir == "" {
		dir = "~/gitee"
	}
	// Uzak kabuk "~"yu tek tırnak içinde genişletmez; $HOME kullanılır.
	hesaplar, err = s.run("cat " + remotePath(dir, "hesaplar.txt"))
	if err != nil {
		return "", "", fmt.Errorf("accounts: hesaplar.txt okunamadı: %w", err)
	}
	patlar, _ = s.run("cat " + remotePath(dir, "patlar.txt")) // PAT opsiyonel
	return hesaplar, patlar, nil
}

// remotePath, uzak kabukta genişleyen bir yol ifadesi üretir.
//
// "~/x" → "$HOME/x" (uzak kabuk genişletir), diğer yollar tek tırnaklanır.
func remotePath(dir, file string) string {
	if strings.HasPrefix(dir, "~/") {
		return `"$HOME/` + strings.TrimPrefix(dir, "~/") + "/" + file + `"`
	}
	if dir == "~" {
		return `"$HOME/` + file + `"`
	}
	return shellQuote(dir + "/" + file)
}

func (s SSHSource) run(remoteCmd string) (string, error) {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if s.Key != "" {
		args = append(args, "-i", s.Key)
	}
	args = append(args, s.Host, remoteCmd)

	out, err := exec.Command("ssh", args...).Output()
	if err != nil {
		msg := ""
		if ee, ok := err.(*exec.ExitError); ok {
			msg = strings.TrimSpace(string(ee.Stderr))
		}
		return "", fmt.Errorf("ssh %s: %v %s", s.Host, err, msg)
	}
	return string(out), nil
}

// Load, SSH'tan çekip ayrıştırılmış hesap havuzunu döndürür.
func Load(s SSHSource) ([]Account, error) {
	h, p, err := s.Fetch()
	if err != nil {
		return nil, err
	}
	accs, err := ParseHesaplar(h)
	if err != nil {
		return nil, err
	}
	AttachPATs(accs, ParsePATs(p))
	return accs, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func envOr(k, def string) string {
	if v, ok := lookupEnv(k); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return def
}

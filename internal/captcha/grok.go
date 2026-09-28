// Package captcha, CAPTCHA çıktığında Grok CLI'ı headless çalıştırıp
// çözdürür.
//
// Canlı doğrulanan çağrı biçimi:
//
//	grok -p "<prompt>" -m <model> --output-format json \
//	     --permission-mode bypassPermissions --always-approve
//
// Yanıt JSON'unda modelUsage alanı gerçek model kimliğini taşır; bu,
// gerçekten o modele gidildiğinin kanıtıdır.
//
// Varsayılan model: workbuddy/global:deepseek-v4.1-flash
package captcha

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Solver, CAPTCHA çözücüdür.
type Solver struct {
	// Bin, grok CLI yolu. Boşsa PATH'te "grok" aranır.
	Bin string
	// Model, kullanılacak model kimliği.
	Model string
	// Timeout, tek çözüm için üst sınır.
	Timeout time.Duration
	// ExtraArgs, ek CLI bayrakları.
	ExtraArgs []string
}

// Default, doğrulanmış varsayılan yapılandırmayı döndürür.
func Default() *Solver {
	bin := os.Getenv("GROK_BIN")
	if bin == "" {
		if h, err := os.UserHomeDir(); err == nil {
			cand := h + "/.grok/bin/grok"
			if _, err := os.Stat(cand); err == nil {
				bin = cand
			}
		}
	}
	if bin == "" {
		bin = "grok"
	}
	model := os.Getenv("GROK_MODEL")
	if model == "" {
		model = "workbuddy/global:deepseek-v4.1-flash"
	}
	// Yidun gibi çok turlu CAPTCHA'lar için tek çözüm 180 sn'yi aşabilir
	// (canlı: görsel okuma + akıl yürütme ~150-290 sn). Varsayılan 420 sn.
	timeout := 420 * time.Second
	if v := os.Getenv("GROK_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			timeout = d
		}
	}
	return &Solver{
		Bin:     bin,
		Model:   model,
		Timeout: timeout,
	}
}

// Request, bir çözüm isteğidir.
type Request struct {
	// Kind, CAPTCHA türü: "image" | "slider" | "text" | "unknown".
	Kind string
	// PageURL, CAPTCHA'nın bulunduğu sayfa.
	PageURL string
	// ImageBase64, varsa CAPTCHA görüntüsü (data: öneki olmadan).
	ImageBase64 string
	// ImagePath, varsa CAPTCHA görüntüsünün disk yolu.
	//
	// CANLI DOĞRULANAN YOL: grok CLI headless, yerel görsel dosyasını Read
	// tool'u ile okuyup analiz edebiliyor (test edildi: 1547x881 PNG doğru
	// tanımlandı). Base64'ü CLI argümanına gömmek "argument list too long"
	// hatası verir; dosya yolu bu yüzden tercih edilir.
	ImagePath string
	// DOMExcerpt, CAPTCHA alanının HTML özeti.
	DOMExcerpt string
	// Instructions, çözümden beklenen biçim.
	Instructions string
}

// Answer, çözüm yanıtıdır.
type Answer struct {
	// Text, modelin döndürdüğü çözüm (ör. "4f8a2c" veya kaydırma koordinatı).
	Text string
	// Raw, ham CLI çıktısı.
	Raw string
	// Model, gerçekten kullanılan model (modelUsage kanıtı).
	Model string
	// Elapsed, çözüm süresi.
	Elapsed time.Duration
}

// Prompt, çözüm isteğini modele verilecek metne çevirir.
func (r Request) Prompt() string {
	var b strings.Builder
	b.WriteString("CAPTCHA çözme görevi. Yalnızca istenen çözümü döndür, açıklama yazma.\n\n")
	if r.Kind != "" {
		fmt.Fprintf(&b, "Tür: %s\n", r.Kind)
	}
	if r.PageURL != "" {
		fmt.Fprintf(&b, "Sayfa: %s\n", r.PageURL)
	}
	if r.Instructions != "" {
		fmt.Fprintf(&b, "Beklenen çıktı: %s\n", r.Instructions)
	}
	if r.DOMExcerpt != "" {
		fmt.Fprintf(&b, "\nCAPTCHA DOM özeti:\n%s\n", r.DOMExcerpt)
	}
	if r.ImagePath != "" {
		fmt.Fprintf(&b, "\nCAPTCHA görsel dosyası (Read tool ile oku): %s\n", r.ImagePath)
	}
	if r.ImageBase64 != "" {
		fmt.Fprintf(&b, "\nGörsel (base64 PNG): %s\n", r.ImageBase64)
	}
	b.WriteString("\nYanıt biçimi: yalnızca çözüm değeri, tek satır.")
	return b.String()
}

// Solve, Grok CLI'ı çalıştırır ve çözümü döndürür.
func (s *Solver) Solve(ctx context.Context, req Request) (*Answer, error) {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{
		"-p", req.Prompt(),
		"-m", s.Model,
		"--output-format", "json",
		"--permission-mode", "bypassPermissions",
		"--always-approve",
	}
	args = append(args, s.ExtraArgs...)

	cmd := exec.CommandContext(cctx, s.Bin, args...)
	cmd.Env = os.Environ()
	start := time.Now()
	out, err := cmd.Output()
	elapsed := time.Since(start)
	if err != nil {
		msg := ""
		if ee, ok := err.(*exec.ExitError); ok {
			msg = strings.TrimSpace(string(ee.Stderr))
		}
		return nil, fmt.Errorf("captcha: grok CLI hatası: %w %s", err, msg)
	}

	ans := &Answer{Raw: string(out), Elapsed: elapsed, Model: s.Model}

	// --output-format json: modelUsage ve metin alanlarını çöz.
	var parsed struct {
		Result     string `json:"result"`
		Text       string `json:"text"`
		Response   string `json:"response"`
		ModelUsage map[string]struct {
			Model string `json:"model"`
		} `json:"modelUsage"`
	}
	if err := json.Unmarshal(out, &parsed); err == nil {
		for k := range parsed.ModelUsage {
			ans.Model = k // gerçek model kimliği kanıtı
			break
		}
		switch {
		case strings.TrimSpace(parsed.Result) != "":
			ans.Text = strings.TrimSpace(parsed.Result)
		case strings.TrimSpace(parsed.Text) != "":
			ans.Text = strings.TrimSpace(parsed.Text)
		case strings.TrimSpace(parsed.Response) != "":
			ans.Text = strings.TrimSpace(parsed.Response)
		}
	}
	if ans.Text == "" {
		ans.Text = cleanPlain(string(out))
	}
	if ans.Text == "" {
		return ans, fmt.Errorf("captcha: model boş yanıt verdi")
	}
	return ans, nil
}

// cleanPlain, JSON olmayan ham çıktıdan tek satırlık yanıtı çıkarır.
func cleanPlain(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			return ln
		}
	}
	return ""
}

// Detect, bir sayfa metninde CAPTCHA işareti arar.
//
// Kahin challenge_status "captcha" derse veya sayfada bilinen CAPTCHA
// sağlayıcılarının izleri varsa true döner.
func Detect(body string) (bool, string) {
	l := strings.ToLower(body)
	marks := []struct{ needle, kind string }{
		{"geetest", "slider"},
		{"captcha", "unknown"},
		{"yidun", "slider"},
		{"nc_1_n1z", "slider"}, // Aliyun NoCaptcha kaydırıcı
		{"recaptcha", "image"},
		{"hcaptcha", "image"},
		{"turnstile", "unknown"},
		{"验证码", "unknown"},
		{"滑块", "slider"},
		{"拖动", "slider"},
	}
	for _, m := range marks {
		if strings.Contains(l, m.needle) {
			return true, m.kind
		}
	}
	return false, ""
}

package sms

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// quackr.io — +86 (Çin) numarası sağlayan sağlayıcı
//
// CANLI DOĞRULANAN (bu oturum):
//   - https://quackr.io/temporary-numbers/china → +86 numaralar listelenir.
//     Örnek: href="/temporary-numbers/china/8615555151447"
//   - Mesajlar client-side yüklenir; API: GET /api/messages/<numara>
//     (?limit=20&timeFilter=86400000). API, Cloudflare Turnstile doğrulaması
//     ister (yalnız tarayıcı oturumunda çalışır; düz HTTP 403 "Verification
//     failed").
//
// Bu yüzden quackr, HTTP ile değil TARAYICI içinde kullanılır:
//   - Numbers: numara listesi HTML'den okunur (sunucu tarafı, çalışır).
//   - Messages: sayfa bağlamında fetch ile okunur (tarayıcı gerekir).
//
// +86 ZORUNLULUĞU: gitcode SMS ucu (POST /api/v1/user/sms/send/codeByBiz)
// YALNIZCA 11 haneli Çin numarası kabul eder. Kanıt (canlı):
//
//	mobile=13800138000  → {"result":true}          (gönderildi)
//	mobile=15555151447  → {"result":true}          (gönderildi)
//	mobile=+8613800138000 → 手机号格式不对          (reddedildi)
//	mobile=+447441913503  → 手机号格式不对          (reddedildi)
//
// Yani +44 gibi uluslararası numaralar SMS ASLA almaz; +86 şarttır.
// ---------------------------------------------------------------------------

// Quackr, quackr.io sağlayıcısıdır.
type Quackr struct {
	Client *http.Client
	// Browser, tarayıcı içi mesaj okuma köprüsüdür (nil ise yalnız numara).
	Browser QuackrBrowser
}

// QuackrBrowser, tarayıcı bağlamında mesaj okuma yeteneğidir.
//
// quackr API'si Cloudflare Turnstile ister; düz HTTP çalışmaz. Bu arayüz,
// Kahin tarayıcısındaki bir eval fonksiyonuyla doldurulur.
type QuackrBrowser interface {
	// FetchMessages, numaranın mesajlarını tarayıcı içinde okur (JSON metni).
	FetchMessages(ctx context.Context, number string) (string, error)
}

func (p *Quackr) Name() string { return "quackr" }

var (
	reQuackrNum = regexp.MustCompile(`href="/temporary-numbers/china/(86[0-9]{9,11})"`)
)

// Numbers, quackr.io'dan +86 numaraları toplar.
func (p *Quackr) Numbers(ctx context.Context) ([]Number, error) {
	body, code, err := Get(ctx, p.Client, "https://quackr.io/temporary-numbers/china")
	if err != nil {
		return nil, fmt.Errorf("quackr: %w", err)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("quackr: HTTP %d", code)
	}
	var out []Number
	seen := map[string]bool{}
	for _, m := range reQuackrNum.FindAllStringSubmatch(body, -1) {
		full := m[1] // 8615555151447
		if seen[full] {
			continue
		}
		seen[full] = true
		// +86 ön eki atılır; Local 11 haneli Çin numarasıdır (1XXXXXXXXXX).
		local := strings.TrimPrefix(full, "86")
		if len(local) != 11 {
			continue // gitcode yalnız 11 hane kabul eder
		}
		out = append(out, Number{
			CC:      "86",
			Local:   local,
			E164:    "+" + full,
			Country: "cn",
			Source:  "quackr",
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("quackr: +86 numara yok")
	}
	return out, nil
}

var reQuackrMsgJSON = regexp.MustCompile(`"content"\s*:\s*"([^"]*)"|"message"\s*:\s*"([^"]*)"|"text"\s*:\s*"([^"]*)"`)

// Messages, numaranın mesajlarını okur.
//
// quackr API'si Turnstile ister; bu yüzden mesaj okuma TARAYICI içinde
// yapılmalıdır (QuackrBrowser). Tarayıcı köprüsü yoksa hata döner —
// sessizce boş liste dönmez.
func (p *Quackr) Messages(ctx context.Context, n Number) ([]Message, error) {
	if p.Browser == nil {
		return nil, fmt.Errorf("quackr: mesaj okuma tarayıcı köprüsü yok (Turnstile gerekir)")
	}
	full := strings.TrimPrefix(n.E164, "+")
	raw, err := p.Browser.FetchMessages(ctx, full)
	if err != nil {
		return nil, fmt.Errorf("quackr: mesaj okuma: %w", err)
	}
	var out []Message
	for _, m := range reQuackrMsgJSON.FindAllStringSubmatch(raw, -1) {
		for i := 1; i < len(m); i++ {
			if m[i] != "" {
				out = append(out, Message{From: "quackr", Text: html.UnescapeString(m[i])})
				break
			}
		}
	}
	return out, nil
}

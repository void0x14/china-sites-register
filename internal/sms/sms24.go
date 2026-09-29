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
// Sms24 — sms24.me
//
// CANLI DOĞRULANAN (bu oturum, tarayıcıda):
//   - https://sms24.me/en/countries/cn → +86 numara listesi.
//     Numara bağlantıları: /en/numbers/<86XXXXXXXXXXX>
//     Örnek: /en/numbers/8613231012150
//   - Numara sayfası: https://sms24.me/en/numbers/8613231012150
//     Sayfa "SMS inbox is ready / Show SMS messages" düğmesi gösterir;
//     düğmeye basınca mesajlar AYNI SAYFADA listelenir (video/ücret yok —
//     canlı doğrulandı, mesajlar okundu: Iris/Fiverr/WalletHub kodları).
//   - Mesaj satırı: "From: <gönderen>" + kod içeren metin.
//
// NOT: Doğrudan /api/messages/... ucu Cloudflare 403 döner (canlı kanıt).
// Bu yüzden okuma sayfa HTML'inden yapılır.
// ---------------------------------------------------------------------------

// Sms24, sms24.me sağlayıcısıdır.
type Sms24 struct {
	Client *http.Client
	// Browser, mesaj okuma köprüsüdür. Sayfa mesajları "Show SMS messages"
	// tıklamasından sonra yüklediği için (canlı kanıt: tıklama öncesi HTML'de
	// mesaj yok, sonrasında DOM'da 5 mesaj) okuma tarayıcı bağlamında yapılır.
	Browser Sms24Browser
}

// Sms24Browser, tarayıcı bağlamında sms24 mesajlarını okuma yeteneğidir.
//
// Dönen metin, sayfadaki "From: X ... <mesaj>" bloklarının düz metnidir.
type Sms24Browser interface {
	// OpenAndReadMessages, numara sayfasını açar, "Show SMS messages"
	// düğmesine basar ve mesaj metnini döndürür.
	OpenAndReadMessages(ctx context.Context, full string) (string, error)
}

func (p *Sms24) Name() string { return "sms24" }

var (
	reS24Num = regexp.MustCompile(`/en/numbers/(86[0-9]{9,11})`)
	reS24Msg = regexp.MustCompile(`(?is)From:\s*([^<]{1,40})</[^>]+>\s*(?:<[^>]+>\s*)*([^<]{5,300})`)
	reS24Any = regexp.MustCompile(`(?is)(?:verification code|code is|验证码|校验码)[^<]{0,60}`)
)

// Numbers, sms24.me'den +86 numaraları toplar.
func (p *Sms24) Numbers(ctx context.Context) ([]Number, error) {
	body, code, err := Get(ctx, p.Client, "https://sms24.me/en/countries/cn")
	if err != nil {
		return nil, fmt.Errorf("sms24: %w", err)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("sms24: HTTP %d", code)
	}
	var out []Number
	seen := map[string]bool{}
	for _, m := range reS24Num.FindAllStringSubmatch(body, -1) {
		full := m[1] // 8613231012150
		if seen[full] {
			continue
		}
		seen[full] = true
		local := strings.TrimPrefix(full, "86")
		// gitcode yalnız 11 haneli Çin numarası kabul eder.
		if len(local) != 11 {
			continue
		}
		out = append(out, Number{
			CC:      "86",
			Local:   local,
			E164:    "+" + full,
			Country: "cn",
			Source:  "sms24",
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("sms24: +86 numara yok")
	}
	return out, nil
}

// Messages, numaranın mesajlarını okur.
//
// CANLI KANIT (bu oturum): mesajlar sayfa HTML'inde YOKTUR; "Show SMS
// messages" düğmesine basıldıktan sonra DOM'a gelirler. Bu yüzden tarayıcı
// köprüsü kullanılır. Köprü yoksa hata döner (sessizce boş liste dönmez —
// aksi hâlde "SMS gelmedi" yanlış sonucu doğar).
func (p *Sms24) Messages(ctx context.Context, n Number) ([]Message, error) {
	if p.Browser == nil {
		return nil, fmt.Errorf("sms24: mesaj okuma tarayıcı köprüsü yok")
	}
	full := strings.TrimPrefix(n.E164, "+")
	raw, err := p.Browser.OpenAndReadMessages(ctx, full)
	if err != nil {
		return nil, fmt.Errorf("sms24: mesaj okuma: %w", err)
	}
	var out []Message
	for _, m := range reS24Block.FindAllStringSubmatch(raw, -1) {
		from := strings.TrimSpace(m[1])
		text := cleanS24(m[2])
		if text == "" {
			continue
		}
		out = append(out, Message{From: from, Text: text})
	}
	// Yedek: "verification code" içeren serbest parçalar.
	if len(out) == 0 {
		for _, t := range reS24Any.FindAllString(raw, -1) {
			out = append(out, Message{From: "sms24", Text: cleanS24(t)})
		}
	}
	return out, nil
}

// reS24Block, DOM düz metnindeki "From: X\n<zaman>\n\n<mesaj>" bloklarını
// yakalar (canlı doğrulandı).
var reS24Block = regexp.MustCompile(`(?is)From:\s*([^\n]{1,40})\s*\n[^\n]*\n\s*\n?\s*([^\n]{5,300})`)

func cleanS24(s string) string {
	s = reTagStrip.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

var reTagStrip = regexp.MustCompile(`(?is)<[^>]+>`)

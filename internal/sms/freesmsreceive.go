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
// free-sms-receive.com — +86 (Çin) numarası veren, mesajları DÜZ HTTP ile
// okunabilen sağlayıcı.
//
// CANLI DOĞRULANAN YOLLAR (bu oturum):
//   - Numara listesi: https://www.free-sms-receive.com/country/china/1.html
//     (https://free-sms-receive.com/... → 301 → www host).
//     Numara bağlantıları: href="/message/8615651173020.html"
//   - Numara sayfası: https://www.free-sms-receive.com/message/8615651173020.html
//     Mesajlar SUNUCU TARAFINDA basılır (JS gerekmez), satır yapısı:
//     <div class="row border-bottom table-hover  bg-messages">
//       <div class="col-xs-12 col-md-2"><div>134588888167</div>...</div>
//       <div class="col-xs-0 col-md-2">7 minutes ago</div>
//       <div class="col-xs-12 col-md-8" style="color:#666464;">【7动】验证码：14178。...</div>
//     </div>
//     Yeni gelen mesaj satırı "bg-messages" sınıfıyla işaretlenir (canlı kanıt).
//
// Bu sağlayıcı quackr'ın aksine TARAYICI GEREKTİRMEZ; kod okuma düz HTTP ile
// çalışır. +86 numaraları paylaşımlıdır (herkes görür), bu yüzden gelen
// mesajda gitcode/中国移动 gibi işaret aranmaz — kod regex'i yeterlidir.
// ---------------------------------------------------------------------------

// FreeSMSReceive, free-sms-receive.com sağlayıcısıdır.
type FreeSMSReceive struct {
	Client *http.Client
	// CountryPath, numara listesi yolu (varsayılan china/1.html).
	CountryPath string
	// CC, numaraların ülke kodu (varsayılan "86").
	CC string
}

func (p *FreeSMSReceive) Name() string { return "free-sms-receive.com" }

var reFSRNumber = regexp.MustCompile(`/message/(86[0-9]{9,11})\.html`)

// fsrRowMark, mesaj satırlarının başlangıç işaretidir.
//
// DİKKAT: satırlar İÇ İÇE div'ler barındırır (gönderen sütunu kendi div'ini
// taşır). Non-greedy `(.*?)</div>` ilk kapanışta kesilir ve mesaj sütunu
// kaybolur (canlı: 20 satır bulundu, 0 mesaj). Bu yüzden satırlar işaret
// noktasından BÖLÜNEREK alınır, regex ile kapatılmaz.
const fsrRowMark = `<div class="row border-bottom table-hover`

// reFSRMsgCol, satır içindeki mesaj sütununu yakalar.
var reFSRMsgCol = regexp.MustCompile(`(?is)<div class="col-xs-12 col-md-8"[^>]*>(.*?)</div>`)

// reFSRFromCol, satır içindeki gönderen numarayı yakalar.
var reFSRFromCol = regexp.MustCompile(`(?is)<div class="col-xs-12 col-md-2">\s*<div[^>]*>([0-9+]+)</div>`)

// baseURL, sağlayıcı kök adresidir (www zorunlu: kök host 301 döner).
const fsrBase = "https://www.free-sms-receive.com"

// Numbers, ülke sayfasındaki +86 numaraları toplar.
func (p *FreeSMSReceive) Numbers(ctx context.Context) ([]Number, error) {
	path := p.CountryPath
	if path == "" {
		path = "country/china/1.html"
	}
	cc := p.CC
	if cc == "" {
		cc = "86"
	}
	body, code, err := Get(ctx, p.Client, fsrBase+"/"+strings.TrimLeft(path, "/"))
	if err != nil {
		return nil, fmt.Errorf("free-sms-receive: %w", err)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("free-sms-receive: HTTP %d", code)
	}
	var out []Number
	seen := map[string]bool{}
	for _, m := range reFSRNumber.FindAllStringSubmatch(body, -1) {
		full := m[1]
		if seen[full] {
			continue
		}
		seen[full] = true
		local := strings.TrimPrefix(full, cc)
		out = append(out, Number{
			CC:      cc,
			Local:   local,
			E164:    "+" + full,
			Country: "cn",
			Source:  p.Name(),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("free-sms-receive: +%s numara yok", cc)
	}
	return out, nil
}

// Messages, numara sayfasındaki mesajları okur.
func (p *FreeSMSReceive) Messages(ctx context.Context, n Number) ([]Message, error) {
	full := strings.TrimPrefix(n.E164, "+")
	if full == "" {
		full = n.Local
	}
	body, code, err := Get(ctx, p.Client, fsrBase+"/message/"+full+".html")
	if err != nil {
		return nil, fmt.Errorf("free-sms-receive: %w", err)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("free-sms-receive: mesaj sayfası HTTP %d", code)
	}
	return parseFSRMessages(body), nil
}

// parseFSRMessages, numara sayfasındaki mesaj satırlarını ayrıştırır.
//
// Satırlar fsrRowMark'tan bölünür; her parça bir sonraki satır işaretine kadar
// olan gövdedir. Mesaj metni col-md-8 sütununda, gönderen col-md-2 sütunundadır.
func parseFSRMessages(body string) []Message {
	var out []Message
	parts := strings.Split(body, fsrRowMark)
	if len(parts) < 2 {
		return out
	}
	for _, part := range parts[1:] {
		// Satır gövdesi: bir sonraki satıra kadar; reklam blokları çok uzun
		// olabilir, ilk 4 KB yeter (mesaj sütunu satırın başında gelir).
		inner := part
		if len(inner) > 4096 {
			inner = inner[:4096]
		}
		mc := reFSRMsgCol.FindStringSubmatch(inner)
		if mc == nil {
			continue
		}
		txt := cleanHTML(mc[1])
		if txt == "" {
			continue
		}
		from := "free-sms-receive.com"
		if fc := reFSRFromCol.FindStringSubmatch(inner); fc != nil {
			from = html.UnescapeString(strings.TrimSpace(fc[1]))
		}
		out = append(out, Message{From: from, Text: txt})
	}
	return out
}

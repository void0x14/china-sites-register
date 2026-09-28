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
// freephonenum.com
//
// Canlı doğrulandı:
//   - Numara listesi: https://freephonenum.com/<cc>/receive-sms  (cc = "us","be",...)
//     Sayfa numara bağlantıları: href="/us/receive-sms/3185456266"
//   - Numara sayfası: https://freephonenum.com/us/receive-sms/3185456266
//     Mesajlar düz metinde okunur, class="msg" bloklarında.
//     Gerçek örnek: "Please, enter phone verification code 807908."
//
// Ücretsiz numaralar çoğunlukla ABD/AB kaynaklıdır; Çin (+86) beklenmemeli.
// ---------------------------------------------------------------------------

// FreePhoneNum, freephonenum.com sağlayıcısıdır.
type FreePhoneNum struct {
	Client *http.Client
	// Countries, taranacak ülke kodları (sırayla).
	Countries []string
}

func (p *FreePhoneNum) Name() string { return "freephonenum" }

var reFPNLink = regexp.MustCompile(`href="/([a-z]{2})/receive-sms/([0-9]+)"`)

// reFPNMsg, freephonenum mesaj metnini yakalar.
//
// DİKKAT: class="msg" div'i iç içe div'ler barındırır (avatar, gönderen,
// metin). Non-greedy `(.*?)</div>` ilk </div>'de kesilip yalnızca avatar
// harfini döndürür. Gerçek mesaj metni class="js-msgtext" div'indedir.
var reFPNMsg = regexp.MustCompile(`(?is)class="[^"]*js-msgtext[^"]*"[^>]*>(.*?)</div>`)

// CCtoE164Prefix, ülke kodlarını E164 ön ekine çevirir.
// freephonenum URL ülke kodları (us, gb, ca, be, fi...) burada eşlenir.
var CCtoE164Prefix = map[string]string{
	"us": "1", "ca": "1", "pr": "1",
	"be": "32", "fi": "358", "nl": "31", "se": "46", "de": "49",
	"fr": "33", "es": "34", "at": "43", "ch": "41", "gb": "44",
	"pl": "48", "ro": "40", "cz": "420", "dk": "45", "no": "47",
	"it": "39", "pt": "351", "ie": "353", "hu": "36", "gr": "30",
	"bg": "359", "za": "27", "nz": "64", "lv": "371", "ee": "372",
}

// Numbers, yapılandırılmış ülkelerden numaraları toplar.
//
// Çalışma anında doğrulanan yollar:
//   - Genel liste : https://freephonenum.com/receive-sms   (tüm ülkeler, ~626 numara)
//   - Ülke listesi: https://freephonenum.com/<cc>          (ör. /us, ~10 numara)
//
// /<cc>/receive-sms yolu 404 döner; kullanılmaz.
func (p *FreePhoneNum) Numbers(ctx context.Context) ([]Number, error) {
	ccs := p.Countries
	if len(ccs) == 0 {
		ccs = []string{"us"}
	}
	var out []Number
	var errs []string
	seen := map[string]bool{}

	// 1) Genel liste: tüm ülkeleri tek istekte verir (us, gb, ca, be, pl...).
	if body, code, err := Get(ctx, p.Client, "https://freephonenum.com/receive-sms"); err != nil {
		errs = append(errs, fmt.Sprintf("global: %v", err))
	} else if code != http.StatusOK {
		errs = append(errs, fmt.Sprintf("global: HTTP %d", code))
	} else {
		out = append(out, parseFPN(body, seen)...)
	}

	// 2) İstenen ülkeler: ülke sayfası (ülke başına ~10 numara).
	for _, cc := range ccs {
		url := fmt.Sprintf("https://freephonenum.com/%s", cc)
		body, code, err := Get(ctx, p.Client, url)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", cc, err))
			continue
		}
		if code != http.StatusOK {
			errs = append(errs, fmt.Sprintf("%s: HTTP %d", cc, code))
			continue
		}
		out = append(out, parseFPN(body, seen)...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("freephonenum: numara yok (%s)", strings.Join(errs, "; "))
	}
	return out, nil
}

// parseFPN, bir freephonenum sayfasındaki numara bağlantılarını toplar.
func parseFPN(body string, seen map[string]bool) []Number {
	var out []Number
	for _, m := range reFPNLink.FindAllStringSubmatch(body, -1) {
		ccURL, digits := m[1], m[2]
		if seen[ccURL+"/"+digits] {
			continue
		}
		seen[ccURL+"/"+digits] = true
		prefix := CCtoE164Prefix[ccURL]
		n := Number{CC: prefix, Local: digits, Country: ccURL, Source: "freephonenum"}
		if prefix != "" && !strings.HasPrefix(digits, prefix) {
			n.E164 = "+" + prefix + digits
		} else {
			n.E164 = "+" + digits
		}
		out = append(out, n)
	}
	return out
}

// Messages, numara sayfasındaki mesajları okur.
func (p *FreePhoneNum) Messages(ctx context.Context, n Number) ([]Message, error) {
	cc := n.Country
	if cc == "" {
		cc = "us"
	}
	local := n.Local
	if local == "" {
		local = strings.TrimPrefix(n.E164, "+")
	}
	url := fmt.Sprintf("https://freephonenum.com/%s/receive-sms/%s", cc, local)
	body, code, err := Get(ctx, p.Client, url)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("freephonenum: mesaj sayfası HTTP %d", code)
	}
	var out []Message
	for _, m := range reFPNMsg.FindAllStringSubmatch(body, -1) {
		txt := cleanHTML(m[1])
		if txt == "" {
			continue
		}
		out = append(out, Message{From: "freephonenum", Text: txt})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// receive-sms-free.cc
//
// Canlı doğrulandı:
//   - Ülke listesi : https://receive-sms-free.cc/Free-<Slug>-Phone-Number/
//     Slug'lar ülke adıdır: "USA", "UK", "Netherlands" ("United-States" 404).
//     Numara bağlantıları: href="/Free-USA-Phone-Number/12543207600/"
//   - Numara sayfası: https://receive-sms-free.cc/Free-<Slug>-Phone-Number/<number>/
//     Mesajlar düz metinde, class="sms-content" bloklarında.
//     Gerçek örnek: "Your verification code is: 184803."
// ---------------------------------------------------------------------------

// ReceiveSMSFreeCC, receive-sms-free.cc sağlayıcısıdır.
type ReceiveSMSFreeCC struct {
	Client *http.Client
	// CountrySlugs, taranacak ülke URL parçaları (ör. "USA", "UK", "Netherlands").
	CountrySlugs []string
}

func (p *ReceiveSMSFreeCC) Name() string { return "receive-sms-free.cc" }

var (
	reRSFCLink = regexp.MustCompile(`href="/Free-([A-Za-z-]+)-Phone-Number/([0-9]+)/"`)
	reRSFCMsg  = regexp.MustCompile(`(?is)<[^>]*class="[^"]*sms-content[^"]*"[^>]*>(.*?)</`)
)

// RSFCSlugCC, receive-sms-free.cc ülke slug'larını ülke koduna eşler.
// Numaralar ülke kodu ön ekiyle gelir (USA: 1..., Netherlands: 31...).
var RSFCSlugCC = map[string]string{
	"USA": "1", "Canada": "1",
	"UK": "44", "Germany": "49", "France": "33", "Italy": "39",
	"Spain": "34", "Poland": "48", "Portugal": "351", "Ireland": "353",
	"Switzerland": "41", "Norway": "47", "Denmark": "45", "Austria": "43",
	"Belgium": "32", "Netherlands": "31", "Sweden": "46", "Finland": "358",
	"Russia": "7", "India": "91", "Malaysia": "60", "Thailand": "66",
}

// Numbers, ülke sayfalarından numaraları toplar.
func (p *ReceiveSMSFreeCC) Numbers(ctx context.Context) ([]Number, error) {
	slugs := p.CountrySlugs
	if len(slugs) == 0 {
		slugs = []string{"USA"}
	}
	var out []Number
	var errs []string
	seen := map[string]bool{}
	for _, slug := range slugs {
		url := fmt.Sprintf("https://receive-sms-free.cc/Free-%s-Phone-Number/", slug)
		body, code, err := Get(ctx, p.Client, url)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", slug, err))
			continue
		}
		if code != http.StatusOK {
			errs = append(errs, fmt.Sprintf("%s: HTTP %d", slug, code))
			continue
		}
		for _, m := range reRSFCLink.FindAllStringSubmatch(body, -1) {
			slugURL, digits := m[1], m[2]
			if seen[slugURL+"/"+digits] {
				continue
			}
			seen[slugURL+"/"+digits] = true
			cc := RSFCSlugCC[slugURL]
			local := digits
			if cc != "" && strings.HasPrefix(digits, cc) {
				local = strings.TrimPrefix(digits, cc)
			}
			out = append(out, Number{
				E164:    "+" + digits,
				CC:      cc,
				Local:   local,
				Country: slugURL,
				Source:  p.Name(),
			})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("receive-sms-free.cc: numara yok (%s)", strings.Join(errs, "; "))
	}
	return out, nil
}

// Messages, numara sayfasındaki mesajları okur.
//
// DİKKAT: bu sağlayıcıda numara sayfası URL'i ülke kodu DAHİL tam numarayı
// kullanır (ör. /Free-USA-Phone-Number/12543207600/), ulusal numarayı değil.
func (p *ReceiveSMSFreeCC) Messages(ctx context.Context, n Number) ([]Message, error) {
	slug := n.Country
	if slug == "" {
		slug = "USA"
	}
	full := strings.TrimPrefix(n.E164, "+")
	if full == "" {
		full = n.Local
	}
	url := fmt.Sprintf("https://receive-sms-free.cc/Free-%s-Phone-Number/%s/", slug, full)
	body, code, err := Get(ctx, p.Client, url)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("receive-sms-free.cc: mesaj sayfası HTTP %d", code)
	}
	var out []Message
	for _, m := range reRSFCMsg.FindAllStringSubmatch(body, -1) {
		txt := cleanHTML(m[1])
		if txt == "" {
			continue
		}
		out = append(out, Message{From: "receive-sms-free.cc", Text: txt})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// ortak
// ---------------------------------------------------------------------------

var (
	reTag     = regexp.MustCompile(`(?s)<[^>]+>`)
	reSpaces  = regexp.MustCompile(`\s+`)
	reTZStamp = regexp.MustCompile(`(?i)\b(?:UTC|GMT)[+-]?\d*\b`)
)

// cleanHTML, etiketleri söker ve boşlukları sadeleştirir.
func cleanHTML(s string) string {
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = reTZStamp.ReplaceAllString(s, " ")
	s = reSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

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
//
// ÇOK ÜLKELİ: hedef formun kabul ettiği tüm ülkelerden numara toplar.
// CANLI KANIT: gitcode SMS'i şu sms24 numaralarına düştü —
// +487911637751 (Polonya), +46729427296 (İsveç), +447576123381 (UK),
// +66660041301 (Tayland), +3584573998041 (Finlandiya), +85256942757 (HK).
// Bu yüzden yalnız +86 değil, formun kabul ettiği ülkelerin TAMAMI taranır.
type Sms24 struct {
	Client *http.Client
	// Countries, taranacak sms24 ülke kodları (ör. "pl","se","fi","gb").
	// Boşsa varsayılan liste kullanılır.
	Countries []string
	// Browser, mesaj okuma köprüsüdür (mesajlar yalnız tarayıcı bağlamında
	// görünür — canlı kanıt: "Show SMS messages" sonrası DOM'a geliyor).
	Browser Sms24Browser
}

// Sms24Browser, tarayıcı bağlamında sms24 mesajlarını okuma yeteneğidir.
type Sms24Browser interface {
	OpenAndReadMessages(ctx context.Context, full string) (string, error)
}

func (p *Sms24) Name() string { return "sms24" }

// sms24 ülke kodu → E.164 ülke kodu eşlemesi.
//
// CANLI DOĞRULANAN: sms24 URL'leri ISO-3166 alpha-2 kullanır
// (https://sms24.me/en/countries/fi). gitcode formunun ülke kodlarıyla
// eşleştirilir.
var sms24Countries = map[string]string{
	"cn": "86", "hk": "852", "tw": "886", "us": "1", "ru": "7",
	"fr": "33", "pt": "351", "ie": "353", "fi": "358", "it": "39",
	"ch": "41", "gb": "44", "se": "46", "no": "47", "pl": "48",
	"de": "49", "ar": "54", "my": "60", "sg": "65", "th": "66",
	"tr": "90", "in": "91", "pk": "92", "il": "972",
}

// sms24DefaultCountries, hiç ülke verilmediğinde taranan ülkelerdir.
// Formun kabul ettiği ve gitcode SMS'inin düştüğü KANITLANMIŞ ülkeler önce.
// sms24Len, ülke koduna göre BEKLENEN toplam numara uzunluğudur
// (ülke kodu + yerel numara). Canlı ölçümle doğrulanmıştır; sms24 bazı
// sayfalarda dahili ID'leri numara gibi listeler (ör. HK'de 14 haneli).
var sms24Len = map[string]int{
	"86":  13, // 86 + 11
	"852": 11, // 852 + 8
	"886": 12, // 886 + 9
	"1":   11, // 1 + 10
	"7":   11, // 7 + 10
	"33":  11, // 33 + 9
	"44":  12, // 44 + 10
	"46":  11, // 46 + 9
	"48":  11, // 48 + 9
	"49":  12, // 49 + 11
	"358": 12, // 358 + 9
	"66":  11, // 66 + 9
	"90":  12, // 90 + 10
}

var sms24DefaultCountries = []string{
	"fi", "pl", "se", "gb", "th", "hk", // gitcode SMS'i düşen ülkeler (kanıtlı)
	"cn", "de", "fr", "it", "us",
}

// Numbers, sms24.me'den (çok ülkeli) numaraları toplar.
func (p *Sms24) Numbers(ctx context.Context) ([]Number, error) {
	countries := p.Countries
	if len(countries) == 0 {
		countries = sms24DefaultCountries
	}
	var out []Number
	var errs []string
	for _, iso := range countries {
		cc, ok := sms24Countries[iso]
		if !ok {
			continue
		}
		nums, err := p.numbersFor(ctx, iso, cc)
		if err != nil {
			errs = append(errs, iso+": "+err.Error())
			continue
		}
		out = append(out, nums...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("sms24: hiç numara yok (%s)", strings.Join(errs, "; "))
	}
	return out, nil
}

// numbersFor, tek bir ülkenin numaralarını toplar.
func (p *Sms24) numbersFor(ctx context.Context, iso, cc string) ([]Number, error) {
	body, code, err := Get(ctx, p.Client, "https://sms24.me/en/countries/"+iso)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", code)
	}
	var out []Number
	seen := map[string]bool{}
	for _, m := range reS24Num.FindAllStringSubmatch(body, -1) {
		full := m[1] // ülke koduna göre tam numara
		if seen[full] {
			continue
		}
		seen[full] = true
		local := strings.TrimPrefix(full, cc)
		if local == "" || local == full {
			continue
		}
		// UZUNLUK DOĞRULAMASI (canlı kanıt): sms24 bazı sayfalarda gerçek
		// numara yerine dahili ID gösterir. Örnek: HK sayfasında
		// "85215976969802" (14 hane) — bu numara DEĞİL. Gerçek HK numarası
		// +852 + 8 hane = 11 hanedir. Bu yüzden ülke bazlı beklenen toplam
		// uzunluk kontrol edilir; uymayan kayıtlar atlanır.
		if want := sms24Len[cc]; want > 0 && len(full) != want {
			continue
		}
		out = append(out, Number{
			CC:      cc,
			Local:   local,
			E164:    "+" + full,
			Country: iso,
			Source:  "sms24",
		})
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

// reS24Num, sms24 numara bağlantılarını yakalar (ülke kodu öneki serbest:
// cn için 86..., fi için 358..., pl için 48...).
var reS24Num = regexp.MustCompile(`/en/numbers/([0-9]{8,15})`)

// reS24Any, serbest metinde kod ipucu arayan yedek desendir.
var reS24Any = regexp.MustCompile(`(?is)(?:verification code|code is|验证码|校验码)[^<]{0,60}`)

func cleanS24(s string) string {
	s = reTagStrip.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

var reTagStrip = regexp.MustCompile(`(?is)<[^>]+>`)

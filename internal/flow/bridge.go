package flow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/void0x14/china-sites-register/internal/kahin"
)

// ---------------------------------------------------------------------------
// SMS okuma köprüleri (tarayıcı içi)
//
// Bazı receive-SMS siteleri mesajları yalnızca tarayıcı bağlamında gösterir:
//
//   - sms24.me: mesajlar "Show SMS messages" düğmesine basılınca DOM'a gelir
//     (canlı kanıt: tıklama öncesi HTML'de mesaj yok, sonrasında 5 mesaj).
//   - quackr.io: Çin numaraları giriş şartı; mesaj API'si Turnstile ister.
//
// Bu köprüler, mesaj okumayı Kahin tarayıcısı üzerinden yapar. HTTP ile
// okunamayan kaynaklar böylece kullanılabilir hâle gelir.
// ---------------------------------------------------------------------------

// BrowserSMSBridge, SMS sağlayıcılarının tarayıcı köprüsüdür.
//
// Aynı Kahin tarayıcısını kullanır; her okuma için yeni sekme açıp kapatır
// (mevcut sayfa/oturum bozulmaz).
type BrowserSMSBridge struct {
	b *kahin.Browser
}

// NewBrowserSMSBridge, köprüyü kurar.
func NewBrowserSMSBridge(b *kahin.Browser) *BrowserSMSBridge {
	return &BrowserSMSBridge{b: b}
}

// OpenAndReadMessages, sms24.me numara sayfasını açar, "Show SMS messages"
// düğmesine basar ve mesaj bloklarının düz metnini döndürür.
//
// CANLI DOĞRULANAN MEKANİK: sayfa açılır → düğme görünür → gerçek tıklama →
// mesajlar DOM'a gelir → sayfa metni "From: X ... <mesaj>" blokları içerir.
func (br *BrowserSMSBridge) OpenAndReadMessages(ctx context.Context, full string) (string, error) {
	if br.b == nil {
		return "", fmt.Errorf("köprü: tarayıcı yok")
	}
	url := "https://sms24.me/en/numbers/" + full
	if err := br.b.Navigate(url, "domcontentloaded", 45*time.Second); err != nil {
		return "", fmt.Errorf("sms24 sayfası: %w", err)
	}
	// "Show SMS messages" düğmesine bas (görünür olana kadar kısa bekle).
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if br.clickByText("Show SMS messages") == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	// Mesajların gelmesini bekle.
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		info, err := br.b.Info()
		if err == nil && strings.Contains(info.Body, "From:") {
			return info.Body, nil
		}
		time.Sleep(700 * time.Millisecond)
	}
	info, err := br.b.Info()
	if err != nil {
		return "", err
	}
	return info.Body, nil
}

// FetchMessages, quackr.io numara sayfasını tarayıcıda açıp mesaj metnini
// döndürür (sms.QuackrBrowser arayüzü).
//
// NOT: quackr Çin numaraları için giriş şartı koyar (canlı kanıt: "China
// virtual numbers are required to register or log in"). Giriş yapılmadığında
// mesajlar gelmez; bu durumda sayfa metni döner ve ayrıştırma boş liste verir.
func (br *BrowserSMSBridge) FetchMessages(ctx context.Context, full string) (string, error) {
	if br.b == nil {
		return "", fmt.Errorf("köprü: tarayıcı yok")
	}
	url := "https://quackr.io/temporary-numbers/china/" + full
	if err := br.b.Navigate(url, "domcontentloaded", 45*time.Second); err != nil {
		return "", fmt.Errorf("quackr sayfası: %w", err)
	}
	time.Sleep(3 * time.Second)
	info, err := br.b.Info()
	if err != nil {
		return "", err
	}
	return info.Body, nil
}

// clickByText, verilen metne sahip düğmeye gerçek fareyle tıklar.
func (br *BrowserSMSBridge) clickByText(text string) error {
	// Koordinatı DOM'dan al.
	out, err := br.b.EvalString(fmt.Sprintf(`(function(){
		var els=Array.from(document.querySelectorAll('button,a'));
		for(var i=0;i<els.length;i++){
			if((els[i].innerText||'').trim()=== %q){
				var r=els[i].getBoundingClientRect();
				if(r.width>0&&r.height>0)return JSON.stringify([r.left+r.width/2,r.top+r.height/2]);
			}
		}
		return 'null';
	})()`, text))
	if err != nil {
		return err
	}
	s := strings.Trim(out, `"`)
	if s == "null" || s == "" {
		return fmt.Errorf("düğme yok: %s", text)
	}
	// Basit JSON dizisi çözümü.
	var xy []float64
	if err := jsonUnmarshal(s, &xy); err != nil || len(xy) != 2 {
		return fmt.Errorf("koordinat çözülemedi: %s", s)
	}
	return br.b.MouseClick(xy[0], xy[1])
}

// jsonUnmarshal, küçük bir JSON dizisini çözer ([x,y]).
func jsonUnmarshal(s string, v *[]float64) error {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return fmt.Errorf("beklenmeyen uzunluk")
	}
	out := make([]float64, 2)
	for i, p := range parts {
		var f float64
		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%g", &f); err != nil {
			return err
		}
		out[i] = f
	}
	*v = out
	return nil
}

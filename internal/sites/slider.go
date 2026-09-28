package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/void0x14/china-sites-register/internal/captcha"
	"github.com/void0x14/china-sites-register/internal/kahin"
)

// ---------------------------------------------------------------------------
// Baidu AFD "curve slider" (b_track_match) çözümü
//
// CANLI DOĞRULANAN MEKANİK (Gitee giriş CAPTCHA'sı):
//   - Tür: Baidu AFD 业务安全风控 → b_track_match. Sayfa metni:
//     "Machine verification" / "Security Verification" /
//     "Drag the slider to match the curve" / "Refresh".
//   - DOM: .session__2verify > .baidu-captcha-container; içinde 11 adet
//     268x178 canvas (katmanlı render), altında sürükleyici ray ve tutamaç.
//   - Görev TEK boyutludur: tutamaç YATAY sürüklenir; tutamaca bağlı yeşil
//     eğri, arka plandaki pembe hedef eğriyle çakışana kadar. Serbest çizim
//     (b_track_draw) DEĞİLDİR.
//   - Sunucu doğrulaması: ofset doğruluğu (is_cap) + davranış/ortam riski
//     (is_spam). Bu yüzden sürükleme gerçek fare olaylarıyla ve insan
//     benzeri yörüngeyle yapılır (Kahin mouse_trajectory).
//
// Çözüm akışı:
//  1. CAPTCHA kutusunun geometrisini oku (canvas + tutamaç, CSS px).
//  2. Ekran görüntüsünü al, canvas bölgesini kırp, diske yaz.
//  3. Grok CLI'ya (captcha.Solver) kırpılmış görselin yolunu ver; yatay
//     ofseti iste.
//  4. Gerçek fare olaylarıyla tutamacı ofset kadar sürükle.
//  5. Başarıyı doğrula; olmadıysa "Refresh" ile yenile ve tekrar dene.
// ---------------------------------------------------------------------------

// sliderGeometry, CAPTCHA'nın ekran üzerindeki yerleşimidir (CSS px).
type sliderGeometry struct {
	Canvas struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		W float64 `json:"w"`
		H float64 `json:"h"`
	} `json:"canvas"`
	Handle struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		W float64 `json:"w"`
		H float64 `json:"h"`
	} `json:"handle"`
	Track struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		W float64 `json:"w"`
	} `json:"track"`
	DPR float64 `json:"dpr"`
}

var reSliderOffset = regexp.MustCompile(`(?i)"?offset_x"?\s*[:=]\s*(-?\d+(?:\.\d+)?)`)

// SolveGiteeSlider, görünen Baidu curve-slider CAPTCHA'sını çözmeyi dener.
//
// Dönen nil, CAPTCHA'nın geçildiği anlamına gelir.
func SolveGiteeSlider(ctx context.Context, b *kahin.Browser, solver *captcha.Solver, pageURL string) error {
	if solver == nil {
		return fmt.Errorf("slider: CAPTCHA çözücü yok (Grok CLI yapılandırılmadı)")
	}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if ok, kind := captchaWall(b); !ok {
			return nil // CAPTCHA yok → geçilmiş
		} else if kind != "slider" {
			return fmt.Errorf("slider: bilinmeyen CAPTCHA türü: %s", kind)
		}

		geo, err := readSliderGeometry(b)
		if err != nil {
			lastErr = err
			time.Sleep(1500 * time.Millisecond)
			continue
		}
		cropPath, err := cropCaptchaCanvas(b, geo, attempt)
		if err != nil {
			lastErr = err
			time.Sleep(1500 * time.Millisecond)
			continue
		}
		offset, err := askSliderOffset(ctx, solver, cropPath, pageURL)
		if err != nil {
			lastErr = err
			time.Sleep(1500 * time.Millisecond)
			continue
		}
		if offset <= 0 {
			// Ofset 0/saçma: CAPTCHA'yı yenile.
			refreshCaptcha(b)
			lastErr = fmt.Errorf("slider: geçersiz ofset %v", offset)
			time.Sleep(1500 * time.Millisecond)
			continue
		}
		if err := dragSlider(b, geo, offset); err != nil {
			lastErr = err
			time.Sleep(1500 * time.Millisecond)
			continue
		}
		// Sonucu doğrula: CAPTCHA kayboldu mu, giriş tamam mı?
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			if ok, _ := captchaWall(b); !ok {
				return nil
			}
			if loginSucceeded(b) {
				return nil
			}
			time.Sleep(1 * time.Second)
		}
		lastErr = fmt.Errorf("slider: sürükleme sonrası CAPTCHA geçilmedi (deneme %d)", attempt)
		refreshCaptcha(b)
		time.Sleep(2 * time.Second)
	}
	return lastErr
}

// readSliderGeometry, CAPTCHA kutusunun geometrisini okur.
func readSliderGeometry(b *kahin.Browser) (*sliderGeometry, error) {
	out, err := b.EvalString(`(function(){
		var c=document.querySelector('.baidu-captcha-container');
		if(!c)return 'null';
		var canv=c.getElementsByTagName('canvas')[0];
		if(!canv)return 'null';
		function r(e){var b=e.getBoundingClientRect();return {x:b.left,y:b.top,w:b.width,h:b.height}};
		var track=c.querySelector('[class*=b514a3f287]');
		var handle=c.querySelector('[class*=b0b2aaec17]');
		var o={canvas:r(canv),dpr:window.devicePixelRatio||1};
		o.track=track?r(track):o.canvas;
		o.handle=handle?r(handle):r(canv);
		return JSON.stringify(o);
	})()`)
	if err != nil {
		return nil, fmt.Errorf("slider geometrisi okunamadı: %w", err)
	}
	s := strings.Trim(out, `"`)
	if s == "null" || s == "" {
		return nil, fmt.Errorf("slider: CAPTCHA kutusu bulunamadı")
	}
	var geo sliderGeometry
	if err := json.Unmarshal([]byte(s), &geo); err != nil {
		return nil, fmt.Errorf("slider geometrisi çözülemedi: %w (%s)", err, truncateOne(s, 160))
	}
	if geo.Canvas.W <= 0 || geo.Handle.W <= 0 {
		return nil, fmt.Errorf("slider: geometri geçersiz")
	}
	return &geo, nil
}

// cropCaptchaCanvas, tam sayfa ekran görüntüsünü alıp canvas bölgesini kırpar.
func cropCaptchaCanvas(b *kahin.Browser, geo *sliderGeometry, attempt int) (string, error) {
	shot, err := b.Screenshot(false)
	if err != nil {
		return "", fmt.Errorf("ekran görüntüsü alınamadı: %w", err)
	}
	f, err := os.Open(shot)
	if err != nil {
		return "", fmt.Errorf("ekran görüntüsü açılamadı: %w", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return "", fmt.Errorf("ekran görüntüsü çözülemedi: %w", err)
	}
	dpr := geo.DPR
	if dpr <= 0 {
		dpr = 1
	}
	// Kırpma payı: eğri kenarlara taşabilir; her yönden %12 pay bırak.
	padX := geo.Canvas.W * 0.12
	padY := geo.Canvas.H * 0.12
	x0 := int((geo.Canvas.X - padX) * dpr)
	y0 := int((geo.Canvas.Y - padY) * dpr)
	x1 := int((geo.Canvas.X + geo.Canvas.W + padX) * dpr)
	y1 := int((geo.Canvas.Y + geo.Canvas.H + padY) * dpr)
	bounds := img.Bounds()
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > bounds.Max.X {
		x1 = bounds.Max.X
	}
	if y1 > bounds.Max.Y {
		y1 = bounds.Max.Y
	}
	if x1 <= x0 || y1 <= y0 {
		return "", fmt.Errorf("slider: kırpma bölgesi geçersiz")
	}
	crop := cropImage(img, image.Rect(x0, y0, x1, y1))
	out := filepath.Join(os.TempDir(), fmt.Sprintf("baidu-slider-%d.png", attempt))
	of, err := os.Create(out)
	if err != nil {
		return "", err
	}
	defer of.Close()
	if err := png.Encode(of, crop); err != nil {
		return "", err
	}
	return out, nil
}

// cropImage, bir görüntünün verilen dikdörtgen bölgesini kopyalar.
//
// image/draw kullanılır: piksel-piksel Set() çağrısı yerine tek çizim;
// büyük kırpımlarda belirgin hız farkı.
func cropImage(src image.Image, r image.Rectangle) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), src, r.Min, draw.Src)
	return dst
}

// askSliderOffset, Grok CLI'ya kırpılmış CAPTCHA görselini verip yatay
// ofseti (görselin kendi piksel uzayında) ister.
func askSliderOffset(ctx context.Context, solver *captcha.Solver, imagePath, pageURL string) (float64, error) {
	defer os.Remove(imagePath) // Grok okuduktan sonra geçici görseli sil
	req := captcha.Request{
		Kind:      "slider",
		PageURL:   pageURL,
		ImagePath: imagePath,
		Instructions: `Baidu "curve slider": yeşil eğri sürükleyici tutamaca bağlıdır; pembe eğri sabit hedeftir. ` +
			`Tutamaç YATAY sürüklenir. Yeşil eğriyi pembe eğrinin üzerine oturtacak yatay kaydırma miktarını, ` +
			`görselin gösterildiği piksel cinsinden ver. Yalnızca şu JSON'u döndür: {"offset_x":<sayı>,"confidence":<0..1>}`,
	}
	ans, err := solver.Solve(ctx, req)
	if err != nil {
		return 0, err
	}
	m := reSliderOffset.FindStringSubmatch(ans.Text)
	if m == nil {
		return 0, fmt.Errorf("slider: Grok yanıtında ofset yok: %s", truncateOne(ans.Text, 200))
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, fmt.Errorf("slider: ofset çözülemedi: %w", err)
	}
	// Ofset, Grok'a verilen kırpılmış görselin piksel uzayındadır; CSS px'e
	// çevrim dragSlider içinde (padFactor × dpr) yapılır.
	return v, nil
}

// dragSlider, tutamacı gerçek fare olaylarıyla yatay olarak sürükler.
//
// Tutamaç CSS px konumundan başlar; hedef = start + ofset. Kahin
// mouse_trajectory insan benzeri (ease-in-out, momentum, sapma) hareket
// üretir — CAPTCHA'nın davranış telemetrisi (is_spam) için gereklidir.
//
// ÖLÇEK: Grok, kırpılmış görselin PİKSEL uzayında ölçer. Kırpım, canvas
// CSS px'inin dpr katı olduğundan (ve %12 pay eklendiğinden) piksel→CSS
// dönüşümü: cssOffset = offset / (padFactor * dpr). Kanlı dpr=1 ekranda
// padFactor=1.24 idi; yüksek DPR'da bu çarpan yanlış sonuç verirdi.
func dragSlider(b *kahin.Browser, geo *sliderGeometry, offset float64) error {
	dpr := geo.DPR
	if dpr <= 0 {
		dpr = 1
	}
	const padFactor = 1.24 // cropCaptchaCanvas: her yönden %12 pay
	cssOffset := offset / (padFactor * dpr)

	// Sürükleme sınırı: tutamaç rayın dışına çıkamaz.
	maxTravel := geo.Track.W - geo.Handle.W
	if maxTravel > 0 && cssOffset > maxTravel {
		cssOffset = maxTravel
	}
	if cssOffset < 1 {
		cssOffset = 1
	}

	startX := geo.Handle.X + geo.Handle.W/2
	startY := geo.Handle.Y + geo.Handle.H/2
	endX := startX + cssOffset

	if err := b.MouseMove(startX, startY); err != nil {
		return fmt.Errorf("fare taşınamadı: %w", err)
	}
	time.Sleep(120 * time.Millisecond)
	if err := b.MouseDown(startX, startY, 0); err != nil {
		return fmt.Errorf("fare basılamadı: %w", err)
	}
	time.Sleep(80 * time.Millisecond)
	if err := b.MouseTrajectory(endX, startY, 0); err != nil {
		return fmt.Errorf("sürükleme yörüngesi: %w", err)
	}
	time.Sleep(150 * time.Millisecond)
	if err := b.MouseUp(endX, startY, 0); err != nil {
		return fmt.Errorf("fare bırakılamadı: %w", err)
	}
	return nil
}

// refreshCaptcha, CAPTCHA'nın "Refresh" düğmesine basar.
func refreshCaptcha(b *kahin.Browser) {
	_, err := b.EvalString(`(function(){
		var c=document.querySelector('.baidu-captcha-container');
		if(!c)return 'yok';
		var el=c.querySelector('[title=Refresh]');
		if(el){el.click();return 'ok'}
		return 'yok';
	})()`)
	_ = err
}

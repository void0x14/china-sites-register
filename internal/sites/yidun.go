package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/void0x14/china-sites-register/internal/captcha"
	"github.com/void0x14/china-sites-register/internal/kahin"
)

// ---------------------------------------------------------------------------
// NetEase Yidun (网易易盾) CAPTCHA çözümü
//
// CANLI DOĞRULANAN MEKANİK (gitcode "Get verification code" sonrası):
//
//   - "Get verification code" tıklaması SMS göndermeden önce NetEase Yidun
//     CAPTCHA'sını açar. Kanıt: tıklama sonrası ağ istekleri
//     c.dun.163.com / cstaticdun.126.net / necaptcha.nosdn.127.net'e gider.
//   - DOM: .yidun_popup (görünür) > .yidun_modal > .yidun_panel >
//     .yidun_bgimg > img.yidun_bg-img (400x300 CSS px, gerçek görsel 480x360),
//     altında .yidun_tips__text (görev metni) ve .yidun_tips__img (hedef şerit).
//   - Görev türleri (canlı görüldü):
//     1) "click in turn"  — alt şeritteki nesnelere SIRAYLA tıkla.
//     2) "swap 2 tiles to restore the image" — iki karoyu takas et.
//     (Zincir çok turludur: bir tur geçilince yenisi gelebilir.)
//   - Çözüm gerçek fare olaylarıyla uygulanır (Kahin mouse_click); tıklama
//     sonrası Yidun numaralı işaretçi koyar ve doğrular.
//
// CANLI KANIT: Grok CLI (workbuddy/global:deepseek-v4.1-flash) görseli okuyup
// nesne koordinatlarını doğru verdi; gerçek tıklamalarla "click in turn"
// turu GEÇİLDİ (yeni tur geldi). Tıklama noktası ile Yidun'un "1" işaretçisi
// örtüştü.
// ---------------------------------------------------------------------------

// yidunVisible, Yidun CAPTCHA popup'ının açık olup olmadığını söyler.
func yidunVisible(b *kahin.Browser) bool {
	return evalBool(b, `(function(){
		var p=document.querySelector('.yidun_popup');
		if(!p)return false;
		var r=p.getBoundingClientRect();
		return r.width>0&&r.height>0&&getComputedStyle(p).display!=='none';
	})()`)
}

// yidunKind, açık turun görev metnini döndürür (küçük harf).
func yidunKind(b *kahin.Browser) string {
	out, err := b.EvalString(`(function(){
		var t=document.querySelector('.yidun_tips__text');
		if(!t)return '';
		return (t.innerText||'').trim().toLowerCase();
	})()`)
	if err != nil {
		return ""
	}
	return strings.Trim(out, `"`)
}

// SolveYidun, açık Yidun CAPTCHA'sını turlar hâlinde çözer.
//
// Dönen nil, popup'ın kapandığı (CAPTCHA'nın geçildiği) anlamına gelir.
func SolveYidun(ctx context.Context, b *kahin.Browser, solver *captcha.Solver, pageURL string) error {
	if solver == nil {
		return fmt.Errorf("yidun: CAPTCHA çözücü yok")
	}
	var lastErr error
	for round := 1; round <= 6; round++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if !yidunVisible(b) {
			return nil
		}
		kind := yidunKind(b)
		imgPath, geo, err := cropYidunImage(b, round)
		if err != nil {
			lastErr = err
			time.Sleep(2 * time.Second)
			continue
		}
		ans, err := askYidun(ctx, solver, imgPath, kind, geo, pageURL)
		if err != nil {
			lastErr = err
			time.Sleep(2 * time.Second)
			continue
		}
		if err := applyYidun(b, kind, ans, geo); err != nil {
			lastErr = err
			time.Sleep(2 * time.Second)
			continue
		}
		// Tur sonucu: popup kapandıysa bitti; yeni tur geldiyse devam.
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if !yidunVisible(b) {
				return nil
			}
			if nk := yidunKind(b); nk != "" && nk != kind {
				break // yeni tur
			}
			time.Sleep(1 * time.Second)
		}
		lastErr = fmt.Errorf("yidun: tur %d geçilemedi (%s)", round, kind)
		// Yenile ve tekrar dene.
		refreshYidun(b)
		time.Sleep(2 * time.Second)
	}
	return lastErr
}

// yidunGeo, Yidun kırpımının ekran yerleşimidir (CSS px) ve kırpılan
// görselin piksel boyutudur (Grok bu uzayda koordinat verir).
//
// Kırpım POPUP'IN TAMAMIDIR (.yidun_modal__body); ana görsel onun içinde
// belirli bir konumdadır. Grok'un verdiği koordinat önce kırpım uzayından
// CSS px'e, sonra ekrana çevrilir.
type yidunGeo struct {
	X, Y, W, H float64 // kırpımın ekran dikdörtgeni (CSS px)
	CropW      float64 // kırpılan görselin piksel genişliği
	CropH      float64 // kırpılan görselin piksel yüksekliği
	// ImgX, ImgY, ImgW, ImgH: ana bulmaca görselinin (img.yidun_bg-img)
	// EKRAN dikdörtgeni. Tıklamalar bu alana düşmelidir.
	ImgX, ImgY, ImgW, ImgH float64
}

// cropYidunImage, Yidun bulmaca görselini ekran görüntüsünden kırpıp diske yazar.
//
// Popup açılırken animasyon nedeniyle öğe boyutu bir an 0 olabilir; bu yüzden
// geometri okuma kısa süre yeniden denenir.
func cropYidunImage(b *kahin.Browser, round int) (string, *yidunGeo, error) {
	var geo yidunGeo
	var ok bool
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		g, err := readYidunGeo(b)
		if err == nil {
			geo, ok = *g, true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !ok {
		return "", nil, fmt.Errorf("yidun görsel geometrisi okunamadı")
	}
	shot, err := b.Screenshot(false)
	if err != nil {
		return "", nil, fmt.Errorf("ekran görüntüsü: %w", err)
	}
	f, err := os.Open(shot)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return "", nil, err
	}
	// Cihaz piksel oranını ekran görüntüsü genişliğinden türet.
	bounds := img.Bounds()
	dpr := 1.0
	if vw, err := b.EvalString(`String(window.innerWidth)`); err == nil {
		if n, e2 := atoi(strings.Trim(vw, `"`)); e2 == nil && n > 0 {
			dpr = float64(bounds.Max.X) / float64(n)
		}
	}
	x0 := int(geo.X * dpr)
	y0 := int(geo.Y * dpr)
	x1 := int((geo.X + geo.W) * dpr)
	y1 := int((geo.Y + geo.H) * dpr)
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
		return "", nil, fmt.Errorf("yidun kırpma bölgesi geçersiz")
	}
	crop := cropImage(img, image.Rect(x0, y0, x1, y1))
	geo.CropW = float64(x1 - x0)
	geo.CropH = float64(y1 - y0)
	p := filepath.Join(os.TempDir(), fmt.Sprintf("yidun-%d.png", round))
	of, err := os.Create(p)
	if err != nil {
		return "", nil, err
	}
	defer of.Close()
	if err := png.Encode(of, crop); err != nil {
		return "", nil, err
	}
	return p, &geo, nil
}

// readYidunGeo, Yidun bulmaca alanının ekran geometrisini okur.
//
// CANLI DOĞRULANAN: Yidun hedef ikonları/karakterleri BAZEN ana görselin alt
// şeridinde (.yidun_bg-img içinde, 480x360), BAZEN ayrı bir şeritte
// (.yidun_tips__img, 320x240) gösterir. Bu yüzden kırpma bölgesi olarak
// POPUP'IN TAMAMI (.yidun_modal__body) alınır: hem ana görsel hem hedef
// şerit hem de "click in turn" metni tek görselde Grok'a gider.
func readYidunGeo(b *kahin.Browser) (*yidunGeo, error) {
	out, err := b.EvalString(`(function(){
		function rect(e){var r=e.getBoundingClientRect();return {x:r.left,y:r.top,w:r.width,h:r.height}};
		var o={};
		// Kırpım alanı: modal gövdesi (ana görsel + hedef şerit + metin).
		var body=document.querySelector('.yidun_modal__body')||document.querySelector('.yidun_panel');
		if(!body)return 'null';
		var rb=rect(body); if(rb.w<=0||rb.h<=0)return 'null';
		o.x=rb.x;o.y=rb.y;o.w=rb.w;o.h=rb.h;
		// Tıklama alanı: ana bulmaca görseli.
		var img=document.querySelector('img.yidun_bg-img')||document.querySelector('.yidun_bgimg');
		if(img){var ri=rect(img);o.imgX=ri.x;o.imgY=ri.y;o.imgW=ri.w;o.imgH=ri.h}
		else {o.imgX=rb.x;o.imgY=rb.y;o.imgW=rb.w;o.imgH=rb.h}
		return JSON.stringify(o);
	})()`)
	if err != nil {
		return nil, err
	}
	s := strings.Trim(out, `"`)
	if s == "null" || s == "" {
		return nil, fmt.Errorf("yidun görseli yok")
	}
	var raw struct {
		X, Y, W, H             float64
		ImgX, ImgY, ImgW, ImgH float64
	}
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, err
	}
	if raw.W <= 0 || raw.H <= 0 {
		return nil, fmt.Errorf("yidun geometrisi geçersiz")
	}
	return &yidunGeo{
		X: raw.X, Y: raw.Y, W: raw.W, H: raw.H,
		ImgX: raw.ImgX, ImgY: raw.ImgY, ImgW: raw.ImgW, ImgH: raw.ImgH,
	}, nil
}

// yidunAnswer, Grok'un verdiği çözümdür.
type yidunAnswer struct {
	// Points, görselin KENDİ piksel uzayında tıklama noktalarıdır.
	Points [][2]float64 `json:"points"`
	// Pairs, takas edilecek karo çiftleri (karo indeksleri, 0 tabanlı satır-major).
	Pairs [][2]int `json:"pairs"`
	// Path, "top sürükle" türü için yol noktalarıdır (görselin kendi uzayı).
	Path [][2]float64 `json:"path"`
	// NatW/NatH, Grok'un kullandığı görsel uzayı (kırpılan görselin boyutu).
	NatW float64 `json:"-"`
	NatH float64 `json:"-"`
}

var (
	reYidunPoints = regexp.MustCompile(`"points"\s*:\s*(\[[^\]]*(?:\][^\]]*)*?\]\s*\])`)
	reYidunPairs  = regexp.MustCompile(`"pairs"\s*:\s*(\[[^\]]*(?:\][^\]]*)*?\]\s*\])`)
	reYidunPath   = regexp.MustCompile(`"path"\s*:\s*(\[[^\]]*(?:\][^\]]*)*?\]\s*\])`)
)

// askYidun, Grok CLI'ya Yidun görselini verip çözümü ister.
func askYidun(ctx context.Context, solver *captcha.Solver, imgPath, kind string, geo *yidunGeo, pageURL string) (*yidunAnswer, error) {
	defer os.Remove(imgPath) // Grok okuduktan sonra geçici görseli sil
	nw, nh := geo.CropW, geo.CropH
	if nw <= 0 {
		nw = 480
	}
	if nh <= 0 {
		nh = 360
	}
	var instr string
	switch {
	case strings.Contains(kind, "drag") || strings.Contains(kind, "ball") || strings.Contains(kind, "obstacle"):
		// CANLI KANIT: "drag the lower left white ball to avoid obstacles and
		// hit ..." — top sol alttan baslar, engellerden kacarak hedefe gider.
		instr = fmt.Sprintf("NetEase Yidun 'drag the ball' CAPTCHA. The image is the WHOLE CAPTCHA popup (%0.fx%0.f). "+
			"A white ball starts at the LOWER LEFT. Drag it upward/rightward, AVOIDING the black obstacles, to reach the goal. "+
			"Give the drag path as waypoints from the ball's start to the goal in the image's OWN pixel space. "+
			`Output ONLY JSON: {"path":[[x,y],[x,y],...],"start":[x,y],"end":[x,y]}`, nw, nh)
	case strings.Contains(kind, "click") || strings.Contains(kind, "turn"):
		instr = fmt.Sprintf("NetEase Yidun 'click in turn' CAPTCHA. The image is the WHOLE CAPTCHA popup: "+
			"the MAIN PHOTO is at the TOP, and BELOW it there is a strip listing the TARGET icons/characters IN ORDER (left to right). "+
			"Find each target (in that order) inside the MAIN PHOTO and give its center as (x,y) in the image's OWN %0.fx%0.f pixel space. "+
			`Output ONLY JSON: {"points":[[x,y],[x,y],...]} with exactly as many points as targets, in order.`, nw, nh)
	case strings.Contains(kind, "swap"):
		instr = fmt.Sprintf("NetEase Yidun 'swap 2 tiles' CAPTCHA. The main photo is a 2x2 grid of tiles (row-major: 0=top-left,1=top-right,2=bottom-left,3=bottom-right); one tile is blank/misplaced. " +
			"Give the two tile indices to swap so the image is restored. " +
			`Output ONLY JSON: {"pairs":[[i,j]]}`)
	default:
		instr = fmt.Sprintf("NetEase Yidun CAPTCHA (task text: %q). Solve it. "+
			`Output ONLY JSON with "points" (list of [x,y] in the image's own %0.fx%0.f pixel space) and/or "pairs".`, kind, nw, nh)
	}
	req := captcha.Request{
		Kind:         "yidun",
		PageURL:      pageURL,
		ImagePath:    imgPath,
		Instructions: instr,
	}
	ans, err := solver.Solve(ctx, req)
	if err != nil {
		return nil, err
	}
	parsed, err := parseYidunAnswer(ans.Text)
	if err != nil {
		return nil, err
	}
	parsed.NatW, parsed.NatH = nw, nh
	return parsed, nil
}

// parseYidunAnswer, Grok metninden nokta/çift listesini çıkarır.
func parseYidunAnswer(text string) (*yidunAnswer, error) {
	out := &yidunAnswer{}
	if m := reYidunPoints.FindStringSubmatch(text); m != nil {
		_ = json.Unmarshal([]byte(m[1]), &out.Points)
	}
	if m := reYidunPairs.FindStringSubmatch(text); m != nil {
		_ = json.Unmarshal([]byte(m[1]), &out.Pairs)
	}
	if m := reYidunPath.FindStringSubmatch(text); m != nil {
		_ = json.Unmarshal([]byte(m[1]), &out.Path)
	}
	if len(out.Points) == 0 && len(out.Pairs) == 0 && len(out.Path) == 0 {
		return nil, fmt.Errorf("yidun: çözüm ayrıştırılamadı: %s", truncateOne(text, 200))
	}
	return out, nil
}

// applyYidun, çözümü gerçek fare olaylarıyla uygular.
//
// Grok, kırpımın (popup gövdesi) piksel uzayında koordinat verir. Bunu
// ekran CSS px'ine çevirip tıklarız:
//
//	screenX = geo.X + (point_x / cropW) * geo.W
//	screenY = geo.Y + (point_y / cropH) * geo.H
func applyYidun(b *kahin.Browser, kind string, ans *yidunAnswer, geo *yidunGeo) error {
	sx := geo.W / ans.NatW
	sy := geo.H / ans.NatH
	if ans.NatW <= 0 || ans.NatH <= 0 {
		sx, sy = 1, 1
	}
	switch {
	case len(ans.Path) > 0:
		// "top sürükle": yol noktalarını sırayla gerçek fare hareketiyle gez.
		// İlk nokta topun başlangıcıdır → mouse_down orada; son nokta hedef
		// → mouse_up. Ara noktalar MouseMove ile.
		first := ans.Path[0]
		sx0 := geo.X + first[0]*sx
		sy0 := geo.Y + first[1]*sy
		if err := b.MouseMove(sx0, sy0); err != nil {
			return err
		}
		time.Sleep(120 * time.Millisecond)
		if err := b.MouseDown(sx0, sy0, 0); err != nil {
			return err
		}
		time.Sleep(80 * time.Millisecond)
		for _, p := range ans.Path[1:] {
			px := geo.X + p[0]*sx
			py := geo.Y + p[1]*sy
			if err := b.MouseMove(px, py); err != nil {
				return err
			}
			time.Sleep(60 * time.Millisecond)
		}
		last := ans.Path[len(ans.Path)-1]
		ex := geo.X + last[0]*sx
		ey := geo.Y + last[1]*sy
		time.Sleep(100 * time.Millisecond)
		if err := b.MouseUp(ex, ey, 0); err != nil {
			return err
		}
	case len(ans.Pairs) > 0:
		// 2x2 karo takası: her karonun merkezine tıkla (ana görsel alanında).
		for _, p := range ans.Pairs {
			for _, idx := range p {
				if idx < 0 || idx > 3 {
					continue
				}
				col := float64(idx % 2)
				row := float64(idx / 2)
				x := geo.ImgX + (col+0.5)*(geo.ImgW/2)
				y := geo.ImgY + (row+0.5)*(geo.ImgH/2)
				if err := b.MouseClick(x, y); err != nil {
					return err
				}
				time.Sleep(700 * time.Millisecond)
			}
		}
	case len(ans.Points) > 0:
		for _, p := range ans.Points {
			x := geo.X + p[0]*sx
			y := geo.Y + p[1]*sy
			// Tıklama ana görsel alanına kırpılır (hedef şeride tıklama sayılmaz).
			if x < geo.ImgX {
				x = geo.ImgX
			}
			if x > geo.ImgX+geo.ImgW {
				x = geo.ImgX + geo.ImgW
			}
			if y < geo.ImgY {
				y = geo.ImgY
			}
			if y > geo.ImgY+geo.ImgH {
				y = geo.ImgY + geo.ImgH
			}
			if err := b.MouseClick(x, y); err != nil {
				return err
			}
			time.Sleep(700 * time.Millisecond)
		}
	default:
		return fmt.Errorf("yidun: uygulanacak çözüm yok")
	}
	return nil
}

// refreshYidun, Yidun'un yenile düğmesine basar.
func refreshYidun(b *kahin.Browser) {
	_, _ = b.EvalString(`(function(){
		var e=document.querySelector('.yidun_refresh');
		if(e){e.click();return 'ok'}
		return 'yok';
	})()`)
}

func atoi(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, fmt.Errorf("boş")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("sayı değil")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

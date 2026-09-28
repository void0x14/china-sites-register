package kahin

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Yüksek seviye tarayıcı sarmalayıcı
//
// Kullanılan tool adları ve argüman şemaları tools/list'ten birebir alındı;
// hepsi canlı çağrı ile doğrulandı (bkz. docs/kahin-mcp-protokol.md §3.4).
// ---------------------------------------------------------------------------

// Browser, tek bir Kahin motoru oturumudur.
type Browser struct {
	c *Client
}

// NewBrowser, istemciyi sarar.
func NewBrowser(c *Client) *Browser { return &Browser{c: c} }

// Start, Camoufox/Mirage motorunu başlatır.
//
// mode: "keş" (varsayılan, izole geçici profil) gibi Kahin modları.
// ephemeral_ack: geçici profil onayı.
func (b *Browser) Start(headless bool, mode string, ephemeralAck bool) (map[string]any, error) {
	if mode == "" {
		mode = "keş"
	}
	res, err := b.c.Call("kahin_browser_start", map[string]any{
		"engine":        "mirage",
		"headless":      headless,
		"mode":          mode,
		"ephemeral_ack": ephemeralAck,
	})
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := res.Decode(&out); err != nil {
		return nil, err
	}
	if code, _ := out["code"].(string); code != "" && code != "ok" {
		return out, fmt.Errorf("kahin: browser_start reddedildi: %s (%v)", code, out["error"])
	}
	return out, nil
}

// Stop, motoru kapatır.
func (b *Browser) Stop() error {
	_, err := b.c.Call("kahin_browser_stop", map[string]any{})
	return err
}

// Health, motor sağlığını döndürür.
func (b *Browser) Health() (map[string]any, error) {
	res, err := b.c.Call("kahin_engine_health", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := res.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// Alive, motorun canlı olup olmadığını söyler.
func (b *Browser) Alive() bool {
	h, err := b.Health()
	if err != nil {
		return false
	}
	alive, _ := h["alive"].(bool)
	return alive
}

// Navigate, sayfayı açar ve sınırlı bir yaşam döngüsü durumunu bekler.
// waitUntil: "commit" | "domcontentloaded" | "load" | "networkidle".
func (b *Browser) Navigate(url, waitUntil string, timeout time.Duration) error {
	if waitUntil == "" {
		waitUntil = "domcontentloaded"
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	_, err := b.c.Call("kahin_navigate", map[string]any{
		"url":        url,
		"wait_until": waitUntil,
		"timeout":    timeout.Seconds(),
	})
	return err
}

// Snapshot, ajan-okunur sayfa gözlemi döndürür (canlı nodeId ref'leriyle).
type Snapshot struct {
	Lines     []string `json:"lines"`
	URL       string   `json:"url"`
	Title     string   `json:"title"`
	ReadyStat string   `json:"readyState"`
	StreamID  string   `json:"streamId"`
	Truncated bool     `json:"truncated"`
}

func (b *Browser) Snapshot(selector string, maxTokens int) (*Snapshot, error) {
	if maxTokens <= 0 {
		maxTokens = 2000
	}
	args := map[string]any{"max_tokens": maxTokens}
	if selector != "" {
		args["selector"] = selector
	}
	res, err := b.c.Call("kahin_mirage_snapshot", args)
	if err != nil {
		return nil, err
	}
	var snap Snapshot
	if err := res.Decode(&snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// Text, snapshot satırlarını tek metne indirger.
func (s *Snapshot) Text() string {
	if s == nil {
		return ""
	}
	return strings.Join(s.Lines, "\n")
}

// ClickSelector, CSS/text/role/xpath locator ile tıklar.
func (b *Browser) ClickSelector(selector string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	res, err := b.c.Call("kahin_mirage_click", map[string]any{
		"selector": selector,
		"timeout":  timeout.Seconds(),
	})
	if err != nil {
		return err
	}
	var out map[string]any
	if err := res.Decode(&out); err != nil {
		return err
	}
	if e, ok := out["error"].(string); ok && e != "" {
		return fmt.Errorf("kahin: click %q: %s", selector, e)
	}
	return nil
}

// Field, fill_form alanıdır. Ref canlı nodeId olmalıdır (snapshot'tan).
type Field struct {
	Ref  string `json:"ref"`
	Text string `json:"text"`
}

// FillForm, canlı nodeId ref'leri üzerinden birden çok alanı doldurur.
// Her alan dom_action(action="type") ile yazılır; bayat ref sahte başarı üretmez.
func (b *Browser) FillForm(fields []Field, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	payload := make([]map[string]any, 0, len(fields))
	for _, f := range fields {
		payload = append(payload, map[string]any{"ref": f.Ref, "text": f.Text})
	}
	res, err := b.c.Call("kahin_mirage_fill_form", map[string]any{
		"fields":  payload,
		"timeout": timeout.Seconds(),
	})
	if err != nil {
		return err
	}
	var out struct {
		Filled  int `json:"filled"`
		Results []struct {
			Target struct {
				NodeID string `json:"nodeId"`
			} `json:"target"`
			RequiresSnapshot bool `json:"requiresSnapshot"`
		} `json:"results"`
	}
	if err := res.Decode(&out); err != nil {
		return err
	}
	if out.Filled != len(fields) {
		return fmt.Errorf("kahin: fill_form %d/%d alan doldurdu (bayat ref olabilir; yeni snapshot al)", out.Filled, len(fields))
	}
	for i, r := range out.Results {
		if r.RequiresSnapshot {
			return fmt.Errorf("kahin: fill_form alan %d (%s) bayat; yeni snapshot gerekli", i, r.Target.NodeID)
		}
	}
	return nil
}

// DOMAction, canlı nodeId üzerinde bir eylem çalıştırır.
// action: "click" | "type" | "check" | "uncheck" | "hover" ...
func (b *Browser) DOMAction(nodeID, action, text string) (map[string]any, error) {
	args := map[string]any{"node_id": nodeID, "action": action}
	if text != "" {
		args["text"] = text
	}
	res, err := b.c.Call("kahin_mirage_dom_action", args)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := res.Decode(&out); err != nil {
		return nil, err
	}
	if e, ok := out["error"].(string); ok && e != "" {
		return out, fmt.Errorf("kahin: dom_action %s/%s: %s", nodeID, action, e)
	}
	return out, nil
}

// Eval, sayfada JavaScript çalıştırır; sonucu JSON olarak döndürür.
//
// DİKKAT: Ana çerçeve ifadeleri izole master world'de çalışır
// (forceScopeAccess). Sayfa-world global'leri görünmez. DOM erişimi vardır.
// Kahin bu sonucu JSON METNİ olarak sarar; burada tekrar çözülür.
func (b *Browser) Eval(expression string) (any, error) {
	res, err := b.c.Call("kahin_mirage_eval", map[string]any{"expression": expression})
	if err != nil {
		return nil, err
	}
	text := res.Text()
	if text == "" {
		return nil, nil
	}
	// Birinci çözümleme: Kahin zarfından çıkan JSON metni.
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return text, nil
	}
	// Eval sonucu bir string ise ve içi JSON ise ikinci kez çöz.
	if s, ok := v.(string); ok {
		var inner any
		if json.Unmarshal([]byte(s), &inner) == nil {
			return inner, nil
		}
		return s, nil
	}
	return v, nil
}

// EvalString, Eval sonucunu string olarak döndürür.
func (b *Browser) EvalString(expression string) (string, error) {
	v, err := b.Eval(expression)
	if err != nil {
		return "", err
	}
	switch t := v.(type) {
	case string:
		return t, nil
	case nil:
		return "", nil
	default:
		b2, _ := json.Marshal(t)
		return string(b2), nil
	}
}

// PageInfo, url/title/body özetidir.
type PageInfo struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Info, geçerli sayfanın url/title/body özetini döndürür.
//
// Hata durumunda nil DEĞİL, boş bir PageInfo ile birlikte hata döner; böylece
// `info, _ := b.Info()` kalıbı nil dereference üretmez (çağıranlar alan
// erişimini hata yolu ayrımı yapmadan kullanıyor).
func (b *Browser) Info() (*PageInfo, error) {
	s, err := b.EvalString(`JSON.stringify({url:location.href,title:document.title,body:document.body?document.body.innerText.slice(0,4000):null})`)
	if err != nil {
		return &PageInfo{}, err
	}
	var info PageInfo
	if err := json.Unmarshal([]byte(s), &info); err != nil {
		return &PageInfo{}, err
	}
	return &info, nil
}

// BodyContains, sayfa gövdesinde bir metin olup olmadığını söyler.
func (b *Browser) BodyContains(sub string) (bool, error) {
	info, err := b.Info()
	if err != nil {
		return false, err
	}
	return strings.Contains(info.Body, sub), nil
}

// Screenshot, ekran görüntüsünü diske yazar ve dosya yolunu döndürür.
//
// CANLI DOĞRULANAN ÇIKTI (kahin_screenshot):
//
//	{"path": "/home/.../screenshot-1790631016665.png", "format": "png", "bytes": 316445}
//
// Eski sürüm base64 alanı arıyordu ve yol döndüğünde JSON metnini "görüntü"
// sanıyordu; bu düzeltildi.
func (b *Browser) Screenshot(fullPage bool) (string, error) {
	res, err := b.c.Call("kahin_screenshot", map[string]any{"full_page": fullPage})
	if err != nil {
		return "", err
	}
	var out struct {
		Path  string `json:"path"`
		Bytes int    `json:"bytes"`
	}
	if err := res.Decode(&out); err != nil {
		return "", fmt.Errorf("kahin: screenshot çıktısı çözülemedi: %w", err)
	}
	if out.Path == "" {
		return "", fmt.Errorf("kahin: screenshot yolu boş: %s", truncateText(res.Text(), 200))
	}
	return out.Path, nil
}

// MouseMove, fareyi görüntü alanı koordinatına taşır (mousemove).
func (b *Browser) MouseMove(x, y float64) error {
	_, err := b.c.Call("kahin_mirage_mouse_move", map[string]any{"x": x, "y": y})
	return err
}

// MouseDown, fare düğmesine basar (mousedown).
func (b *Browser) MouseDown(x, y float64, button int) error {
	_, err := b.c.Call("kahin_mirage_mouse_down", map[string]any{"x": x, "y": y, "button": button})
	return err
}

// MouseUp, fare düğmesini bırakır (mouseup).
func (b *Browser) MouseUp(x, y float64, button int) error {
	_, err := b.c.Call("kahin_mirage_mouse_up", map[string]any{"x": x, "y": y, "button": button})
	return err
}

// MouseClick, görüntü alanı koordinatına gerçek fare tıklaması yapar.
func (b *Browser) MouseClick(x, y float64) error {
	_, err := b.c.Call("kahin_mirage_mouse_click", map[string]any{"x": x, "y": y})
	return err
}

// MouseTrajectory, fareyi son konumdan (x,y)'ye insan benzeri tek hamleyle taşır.
//
// Kahin bunu gerçek mesafe-uyarlamalı adım sayısı, ease-in-out hız profili,
// momentum ve organik sapma ile yapar; insan-benzeri sürükleme (CAPTCHA
// davranış telemetrisi) için bu kullanılır.
func (b *Browser) MouseTrajectory(x, y float64, steps int) error {
	_, err := b.c.Call("kahin_mirage_mouse_trajectory", map[string]any{
		"x":     x,
		"y":     y,
		"steps": steps,
	})
	return err
}

// DragXY, gerçek fare olaylarıyla (x1,y1) noktasından (x2,y2) noktasına sürükler.
//
// steps: ara taşıma adımı sayısı (0 → Kahin mesafeye göre uyarlar).
func (b *Browser) DragXY(x1, y1, x2, y2 float64, steps int) error {
	if err := b.MouseMove(x1, y1); err != nil {
		return err
	}
	if err := b.MouseDown(x1, y1, 0); err != nil {
		return err
	}
	if err := b.MouseTrajectory(x2, y2, steps); err != nil {
		return err
	}
	return b.MouseUp(x2, y2, 0)
}

func truncateText(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ---------------------------------------------------------------------------
// Snapshot ayrıştırma
//
// Snapshot satırları: `- textbox "Phone" [ref=n43vjxa7u-226] [focus, type]`
// ---------------------------------------------------------------------------

var (
	reSnapLine = regexp.MustCompile(`^\s*-\s+([a-zA-Z]+)\s+"([^"]*)"\s+\[ref=([^\]]+)\](.*)$`)
	reSnapAnon = regexp.MustCompile(`^\s*-\s+([a-zA-Z]+)\s+\[ref=([^\]]+)\](.*)$`)
)

// SnapNode, snapshot'taki tek bir düğümdür.
type SnapNode struct {
	Role  string // textbox, button, link, checkbox ...
	Label string
	Ref   string // canlı nodeId
	Flags string // "[click]", "[focus, type]" ...
	Line  string
}

// ParseSnapshot, snapshot satırlarını düğümlere ayırır.
func ParseSnapshot(lines []string) []SnapNode {
	out := make([]SnapNode, 0, len(lines))
	for _, ln := range lines {
		if m := reSnapLine.FindStringSubmatch(ln); m != nil {
			out = append(out, SnapNode{Role: m[1], Label: m[2], Ref: m[3], Flags: m[4], Line: ln})
			continue
		}
		if m := reSnapAnon.FindStringSubmatch(ln); m != nil {
			out = append(out, SnapNode{Role: m[1], Ref: m[2], Flags: m[3], Line: ln})
		}
	}
	return out
}

// FindRole, verilen role ve etiket parçasına uyan ilk düğümü döndürür.
// label boşsa role uyan ilk düğüm döner.
func FindRole(nodes []SnapNode, role, label string) *SnapNode {
	for i := range nodes {
		if !strings.EqualFold(nodes[i].Role, role) {
			continue
		}
		if label == "" || strings.Contains(nodes[i].Label, label) {
			return &nodes[i]
		}
	}
	return nil
}

// FindAllRole, role uyan tüm düğümleri döndürür.
func FindAllRole(nodes []SnapNode, role string) []SnapNode {
	var out []SnapNode
	for _, n := range nodes {
		if strings.EqualFold(n.Role, role) {
			out = append(out, n)
		}
	}
	return out
}

// FindClickable, verilen etiketi içeren tıklanabilir ilk düğümü döndürür.
func FindClickable(nodes []SnapNode, label string) *SnapNode {
	for i := range nodes {
		if !strings.Contains(nodes[i].Label, label) {
			continue
		}
		if strings.Contains(nodes[i].Flags, "click") {
			return &nodes[i]
		}
	}
	return nil
}

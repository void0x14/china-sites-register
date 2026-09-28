// Package sms, receive-SMS sağlayıcılarından telefon numarası alır ve
// o numaraya gelen SMS kodunu okur.
//
// Katalog: repo kökündeki FREE-RECEIVE-SMS-KATALOG.md.
//
// Tasarım: sağlayıcılar "Provider" arayüzünü uygular; havuz (Pool) sırayla
// dener ve ilk çalışan numarayı döndürür. Tüm ağ erişimi net/http iledir;
// tarayıcı gerektiren sağlayıcılar Kahin'e delege edilir (Provider.Browser).
package sms

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// randIntn, [0,n) aralığında kriptografik rastgele sayı döndürür.
// Numara seçimi paylaşımlı havuzda tekrarı önlemek için rastgeleleştirilir.
func randIntn(n int) int {
	if n <= 1 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

// Number, alınan bir telefon numarasıdır.
type Number struct {
	E164    string // +<ülke><numara>
	CC      string // ülke kodu, ör. "86"
	Local   string // ülke kodu olmadan
	Country string // ülke adı/kodu, ör. "us"
	Source  string // sağlayıcı adı
}

// Message, gelen bir SMS'tir.
type Message struct {
	From string
	Text string
	At   time.Time
}

// Provider, tek bir receive-SMS kaynağıdır.
type Provider interface {
	// Name, sağlayıcı adı.
	Name() string
	// Numbers, geçerli numaraları listeler.
	Numbers(ctx context.Context) ([]Number, error)
	// Messages, bir numaraya gelen mesajları okur.
	Messages(ctx context.Context, n Number) ([]Message, error)
}

// ---------------------------------------------------------------------------
// HTTP yardımcıları
// ---------------------------------------------------------------------------

// BrowserUA, tarayıcı kimliğidir. Basit bot filtrelerini geçmek için
// sağlayıcıların çoğu gerçek bir UA ister.
const BrowserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// NewHTTPClient, makul zaman aşımlı bir istemci döndürür.
func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: 25 * time.Second}
}

// Get, UA'lı bir GET yapar ve gövdeyi döndürür.
func Get(ctx context.Context, c *http.Client, url string) (string, int, error) {
	if c == nil {
		c = NewHTTPClient()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", BrowserUA)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := c.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	buf := make([]byte, 0, 1<<16)
	tmp := make([]byte, 32<<10)
	for {
		n, rerr := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if len(buf) > 8<<20 {
				break
			}
		}
		if rerr != nil {
			break
		}
	}
	return string(buf), resp.StatusCode, nil
}

// ---------------------------------------------------------------------------
// Havuz
// ---------------------------------------------------------------------------

// Pool, sıralı sağlayıcı denemesidir.
type Pool struct {
	Providers []Provider
	Client    *http.Client
	// PerProviderTimeout, tek sağlayıcı için üst sınır.
	PerProviderTimeout time.Duration
}

// NewPool, verilen sağlayıcılarla havuz kurar.
func NewPool(ps ...Provider) *Pool {
	return &Pool{
		Providers:          ps,
		Client:             NewHTTPClient(),
		PerProviderTimeout: 40 * time.Second,
	}
}

// AcquireResult, numara alma denemesinin sonucudur.
type AcquireResult struct {
	Number   Number
	Provider string
	Err      error
	Attempts []string
}

// Acquire, sırayla sağlayıcıları dener; ilk uygun numarayı döndürür.
//
// preferredCC boş değilse o ülke kodu önceliklendirilir (ör. "86" Çin).
// allowedCCs boş değilse yalnızca o ülke kodlarındaki numaralar kabul edilir
// (hedef formun ülke listesiyle uyum için). Uygun numara yoksa sağlayıcı atlanır.
//
// excludeCC verilirse o ülke kodundaki numaralar atlanır (numaranın ülkesi
// hedef form tarafından kabul edilmiyorsa gereksiz deneme yapılmaz).
func (p *Pool) Acquire(ctx context.Context, preferredCC string, allowedCCs ...string) (*AcquireResult, error) {
	return p.AcquireExcluding(ctx, preferredCC, nil, allowedCCs...)
}

// AcquireExcluding, daha önce denenmiş numaraları hariç tutarak numara seçer.
//
// exclude: E164 → true. Tüm numaralar dışlanmışsa dışlama gevşetilir
// (yine de numara döner) — çağıran katman başarısızlıkta farklı numara
// istemek için seen kümesini büyütür.
func (p *Pool) AcquireExcluding(ctx context.Context, preferredCC string, exclude map[string]bool, allowedCCs ...string) (*AcquireResult, error) {
	res := &AcquireResult{}
	for _, prov := range p.Providers {
		timeout := p.PerProviderTimeout
		if timeout <= 0 {
			timeout = 40 * time.Second
		}
		pctx, cancel := context.WithTimeout(ctx, timeout)
		nums, err := prov.Numbers(pctx)
		cancel()
		if err != nil {
			res.Attempts = append(res.Attempts, fmt.Sprintf("%s: hata: %v", prov.Name(), err))
			continue
		}
		if len(nums) == 0 {
			res.Attempts = append(res.Attempts, fmt.Sprintf("%s: numara yok", prov.Name()))
			continue
		}
		eligible := filterCC(nums, allowedCCs)
		if len(eligible) == 0 {
			res.Attempts = append(res.Attempts, fmt.Sprintf("%s: %d numara ama uygun ülke yok (%s)", prov.Name(), len(nums), strings.Join(allowedCCs, ",")))
			continue
		}
		pick, why := pickNumberReason(eligible, preferredCC, exclude)
		res.Attempts = append(res.Attempts, fmt.Sprintf("%s: %d numara, %d uygun, seçilen %s (%s)", prov.Name(), len(nums), len(eligible), pick.E164, why))
		res.Number = pick
		res.Provider = prov.Name()
		return res, nil
	}
	res.Err = fmt.Errorf("sms: hiçbir sağlayıcıdan numara alınamadı (%s)", strings.Join(res.Attempts, "; "))
	return res, res.Err
}

// filterCC, numaraları izin verilen ülke kodlarına göre süzer.
// allowed boşsa tümü geçer.
func filterCC(nums []Number, allowed []string) []Number {
	if len(allowed) == 0 {
		return nums
	}
	set := make(map[string]bool, len(allowed))
	for _, cc := range allowed {
		set[strings.TrimPrefix(cc, "+")] = true
	}
	var out []Number
	for _, n := range nums {
		if set[strings.TrimPrefix(n.CC, "+")] {
			out = append(out, n)
		}
	}
	return out
}

// pickNumber, tercih edilen ülke koduna göre numara seçer.
//
// Ücretsiz receive-SMS numaraları PAYLAŞIMLIDIR: aynı numara tekrar seçilirse
// ya "yanmış" (başkası kullanmış, kodu tükenmiş) olur ya da aynı numaraya
// art arda kod istenmesi hedef sitede reddedilir. Bu yüzden seçim rastgele
// yapılır ve daha önce kullanılmış numaralar tercih edilmez.
//
// Dönen ikinci değer: seçimin nedeni (günlük için).
func pickNumber(nums []Number, cc string) Number {
	n, _ := pickNumberReason(nums, cc, nil)
	return n
}

// pickNumberReason, hariç tutulan küme verilerek numara seçer.
func pickNumberReason(nums []Number, cc string, exclude map[string]bool) (Number, string) {
	pool := nums
	if cc != "" {
		cc = strings.TrimPrefix(cc, "+")
		var filtered []Number
		for _, n := range nums {
			if strings.TrimPrefix(n.CC, "+") == cc {
				filtered = append(filtered, n)
			}
		}
		if len(filtered) > 0 {
			pool = filtered
		}
	}
	if len(exclude) > 0 {
		var fresh []Number
		for _, n := range pool {
			if !exclude[n.E164] {
				fresh = append(fresh, n)
			}
		}
		if len(fresh) > 0 {
			return fresh[randIntn(len(fresh))], "taze"
		}
	}
	return pool[randIntn(len(pool))], "rastgele"
}

// WaitForCode, bir numaraya gelen SMS metinlerini yoklar ve kodu çıkarır.
//
// Döngü: her interval'da sağlayıcıdan mesajlar okunur, seen'de olmayan yeni
// mesajlarda kod regex'i aranır. Bulunan ilk kod döner.
func (p *Pool) WaitForCode(ctx context.Context, prov Provider, n Number, seen map[string]bool, timeout, interval time.Duration) (string, string, error) {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	if interval <= 0 {
		interval = 8 * time.Second
	}
	if seen == nil {
		seen = map[string]bool{}
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		default:
		}
		pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		msgs, err := prov.Messages(pctx, n)
		cancel()
		if err != nil {
			lastErr = err
		} else {
			for _, m := range msgs {
				key := m.From + "|" + m.Text
				if seen[key] {
					continue
				}
				seen[key] = true
				if code := ExtractCode(m.Text); code != "" {
					return code, m.Text, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(interval):
		}
	}
	if lastErr != nil {
		return "", "", fmt.Errorf("sms: kod zaman aşımı (%v), son hata: %w", timeout, lastErr)
	}
	return "", "", fmt.Errorf("sms: kod zaman aşımı (%v)", timeout)
}

// ---------------------------------------------------------------------------
// Kod çıkarma
// ---------------------------------------------------------------------------

var (
	reCodeLabeled = regexp.MustCompile(`(?i)(?:code|otp|pin|verification|doğrulama|kodu|kod)\D{0,20}(\d{4,8})`)
	reCodeDigits  = regexp.MustCompile(`\b(\d{4,8})\b`)
	reCodeGitee   = regexp.MustCompile(`(\d{6})`)
)

// ExtractCode, SMS metninden doğrulama kodunu çıkarır.
//
// Sıra: etiketli kod (code/OTP/kod ...) → 6 haneli → 4-8 haneli ilk sayı.
// Bu sıra, gerçek mesajlardaki gürültüyü (tarih, tutar) elemez ama öne alır.
func ExtractCode(text string) string {
	if text == "" {
		return ""
	}
	if m := reCodeLabeled.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	if m := reCodeGitee.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	if m := reCodeDigits.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return ""
}

// ---------------------------------------------------------------------------
// Yardımcı: aynı anda birden çok numarayı izleme
// ---------------------------------------------------------------------------

// Watcher, bir numarayı arka planda izler ve ilk kodu kanala yazar.
type Watcher struct {
	Pool     *Pool
	Provider Provider
	Number   Number
	Interval time.Duration

	mu   sync.Mutex
	seen map[string]bool
}

// Watch, ctx iptal edilene veya kod bulunana kadar izler.
func (w *Watcher) Watch(ctx context.Context, timeout time.Duration) (string, string, error) {
	w.mu.Lock()
	if w.seen == nil {
		w.seen = map[string]bool{}
	}
	w.mu.Unlock()
	return w.Pool.WaitForCode(ctx, w.Provider, w.Number, w.seen, timeout, w.Interval)
}

// ---------------------------------------------------------------------------
// Numara normalleştirme
// ---------------------------------------------------------------------------

var reNonDigit = regexp.MustCompile(`\D`)

// NormalizeNumber, bir numarayı E164'e çevirir.
func NormalizeNumber(raw, cc string) Number {
	digits := reNonDigit.ReplaceAllString(raw, "")
	if strings.HasPrefix(digits, "00") {
		digits = digits[2:]
	}
	n := Number{Local: digits, CC: cc, Source: ""}
	if cc != "" && !strings.HasPrefix(digits, cc) {
		n.E164 = "+" + cc + digits
	} else if cc != "" {
		n.E164 = "+" + digits
	} else {
		n.E164 = "+" + digits
	}
	return n
}

// SortByCC, numaraları ülke koduna göre gruplar (kararlı çıktı için).
func SortByCC(nums []Number) []Number {
	out := make([]Number, len(nums))
	copy(out, nums)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CC != out[j].CC {
			return out[i].CC < out[j].CC
		}
		return out[i].E164 < out[j].E164
	})
	return out
}

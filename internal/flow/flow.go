// Package flow, uçtan uca kayıt akışını yönetir:
//
//	Gitee hesabı (laptop) → hedef sitede "Gitee ile kayıt ol" →
//	telefon numarası (receive-SMS) → SMS kodu → kayıt tamamla
//
// CAPTCHA çıkarsa captcha.Solver (Grok CLI) devreye girer.
package flow

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/void0x14/china-sites-register/internal/accounts"
	"github.com/void0x14/china-sites-register/internal/captcha"
	"github.com/void0x14/china-sites-register/internal/kahin"
	"github.com/void0x14/china-sites-register/internal/sites"
	"github.com/void0x14/china-sites-register/internal/sms"
)

// Options, akış yapılandırmasıdır.
type Options struct {
	Accounts    []accounts.Account
	Site        sites.Site
	Pool        *sms.Pool
	Browser     *kahin.Browser
	Solver      *captcha.Solver
	PreferredCC string   // tercih edilen ülke kodu (ör. "86")
	AllowedCCs  []string // yalnızca bu ülke kodlarındaki numaralar kabul edilir
	CodeTimeout time.Duration
	// MaxPhoneAttempts, bir hesap için en fazla kaç farklı numara denenecek.
	// 0 ise 1 (tek numara).
	MaxPhoneAttempts int
	// Log, satır bazlı günlükleyici. Boşsa stdout.
	Log func(string)
}

// Report, akışın toplu sonucudur.
type Report struct {
	Results  []*sites.Result
	Started  time.Time
	Finished time.Time
}

// OK, başarılı kayıt sayısıdır.
func (r *Report) OK() int {
	n := 0
	for _, x := range r.Results {
		if x.Success {
			n++
		}
	}
	return n
}

// Run, tüm hesaplar için sırayla kayıt dener.
//
// Sıralı çalışır: tek tarayıcı slotu ve tek SMS numarası akışı vardır.
func Run(ctx context.Context, o Options) *Report {
	if o.Log == nil {
		o.Log = func(s string) { fmt.Fprintln(os.Stdout, s) }
	}
	if o.CodeTimeout <= 0 {
		o.CodeTimeout = 5 * time.Minute
	}
	if o.MaxPhoneAttempts <= 0 {
		o.MaxPhoneAttempts = 1
	}
	rep := &Report{Started: time.Now()}
	defer func() { rep.Finished = time.Now() }()

	// Paylaşımlı receive-SMS numaraları "yanar" (başkası kullanmış, kod gelmez).
	// Bir numara ile kayıt tamamlanmazsa FARKLI bir numara ile yeniden denenir.
	usedPhones := map[string]bool{}

	for i := range o.Accounts {
		acc := o.Accounts[i]
		select {
		case <-ctx.Done():
			o.Log("! bağlam iptal edildi, kalan hesaplar atlandı")
			return rep
		default:
		}
		o.Log(fmt.Sprintf("== [%d/%d] %s (%s)", i+1, len(o.Accounts), acc.Username, acc.Email))

		var res *sites.Result
		for attempt := 1; attempt <= o.MaxPhoneAttempts; attempt++ {
			select {
			case <-ctx.Done():
				return rep
			default:
			}
			res = runOne(ctx, o, acc, usedPhones)
			if res.Success {
				break
			}
			// SMS kaynaklı başarısızlıkta (kod gelmedi/numara yandı) farklı
			// numara dene. Diğer aşamalarda (giriş, form) numara değiştirmek
			// anlamsızdır.
			if !retryableStage(res.Stage) {
				break
			}
			if res.Phone.E164 != "" {
				usedPhones[res.Phone.E164] = true
			}
			if attempt < o.MaxPhoneAttempts {
				o.Log(fmt.Sprintf("   ↻ farklı numara ile yeniden denenecek (%d/%d)", attempt+1, o.MaxPhoneAttempts))
			}
		}
		rep.Results = append(rep.Results, res)
		if res.Success {
			o.Log(fmt.Sprintf("   ✓ kayıt başarılı | %s | %s", res.Phone.E164, res.Evidence))
		} else {
			o.Log(fmt.Sprintf("   ✗ %s aşamasında durdu: %v", res.Stage, res.Err))
		}
	}
	return rep
}

// retryableStage, numara değiştirip yeniden denemenin anlamlı olduğu aşamayı söyler.
func retryableStage(stage string) bool {
	switch stage {
	case "gitcode_kayıt", "numara_alma":
		return true
	default:
		return false
	}
}

func runOne(ctx context.Context, o Options, acc accounts.Account, exclude map[string]bool) *sites.Result {
	res := &sites.Result{Site: o.Site.Name(), Account: acc}

	// 1) Numara al. Yalnızca hedef formun kabul ettiği ülke kodları seçilir;
	// aksi halde numara forma girilemez. Daha önce denenmiş numaralar atlanır.
	acq, err := o.Pool.AcquireExcluding(ctx, o.PreferredCC, exclude, o.AllowedCCs...)
	if acq != nil {
		for _, a := range acq.Attempts {
			o.Log("   sms: " + a)
		}
	}
	if err != nil {
		res.Stage = "numara_alma"
		res.Err = err
		return res
	}
	res.Phone = acq.Number
	o.Log("   numara: " + acq.Number.E164 + " (" + acq.Provider + ")")

	// 2) SMS kodu okuyucu: numaraya gelen kodu bekler.
	//
	// KRİTİK: ücretsiz receive-SMS numaralarının mesajları herkese açıktır ve
	// eski mesajlar listede durur. Kod istemeden önce mevcut mesajlar "görüldü"
	// işaretlenir; aksi halde eski bir kod yeni sanılıp gönderilir.
	prov := findProvider(o.Pool, acq.Provider)
	seen := map[string]bool{}
	seedSeen(ctx, prov, acq.Number, seen)
	codeFn := func(cctx context.Context, n sms.Number) (string, error) {
		o.Log("   kod bekleniyor (en çok " + o.CodeTimeout.String() + ")...")
		code, text, err := o.Pool.WaitForCode(cctx, prov, n, seen, o.CodeTimeout, 8*time.Second)
		if err != nil {
			return "", err
		}
		o.Log("   SMS: " + truncate(text, 140))
		return code, nil
	}

	// 3) Kayıt akışı.
	//
	// CAPTCHA artık adaptör katmanında çözülür (Gitee girişinde çıkarsa
	// GitCode.Register → SolveGiteeSlider → Grok CLI). SMS kodu beklerken
	// ayrıca gözlem yapılmaz; eski wrapWithCaptcha yalnızca kod beklerken
	// sayfayı izliyordu ve CAPTCHA'yı çözmüyordu.
	res, err = o.Site.Register(ctx, o.Browser, acc, acq.Number, codeFn)
	if err != nil && res != nil {
		res.Err = err
	}
	if res == nil {
		res = &sites.Result{Site: o.Site.Name(), Account: acc, Phone: acq.Number, Err: err, Stage: "bilinmeyen"}
	}
	return res
}

// wrapWithCaptcha kaldırıldı: CAPTCHA artık adaptör katmanında (sites)
// gerçekten çözülür; buradaki eski sürüm yalnızca gözlem yapıyordu.

func findProvider(p *sms.Pool, name string) sms.Provider {
	for _, prov := range p.Providers {
		if prov.Name() == name {
			return prov
		}
	}
	if len(p.Providers) > 0 {
		return p.Providers[0]
	}
	return nil
}

// seedSeen, numarada zaten var olan mesajları "görüldü" işaretler.
//
// Böylece kod beklerken yalnızca yeni gelen mesajlar değerlendirilir; eski
// (başka biri tarafından kullanılmış) bir kod yanlışlıkla alınmaz.
func seedSeen(ctx context.Context, prov sms.Provider, n sms.Number, seen map[string]bool) {
	if prov == nil {
		return
	}
	pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	msgs, err := prov.Messages(pctx, n)
	if err != nil {
		return
	}
	for _, m := range msgs {
		seen[m.From+"|"+m.Text] = true
	}
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ---------------------------------------------------------------------------
// Sağlık kontrolü
// ---------------------------------------------------------------------------

// Doctor, ortamın akış için hazır olup olmadığını denetler.
type Doctor struct {
	KahinConfig  kahin.RuntimeConfig
	KahinTools   kahin.Toolset
	KahinAlive   bool
	GrokBin      string
	GrokModel    string
	SSHHost      string
	AccountsOK   int
	AccountsErr  error
	ProvidersOK  []string
	ProvidersErr []string
	Notes        []string
}

// Diagnose, tüm bileşenleri sırayla kontrol eder.
func Diagnose(ctx context.Context, sshHost string, pool *sms.Pool, solver *captcha.Solver) *Doctor {
	d := &Doctor{}
	cfg, err := kahin.Resolve()
	if err != nil {
		d.Notes = append(d.Notes, "kahin çözümlenemedi: "+err.Error())
	}
	d.KahinConfig = cfg
	if solver != nil {
		d.GrokBin = solver.Bin
		d.GrokModel = solver.Model
	}
	d.SSHHost = sshHost

	src := accounts.DefaultSSHSource()
	if sshHost != "" {
		src.Host = sshHost
	}
	d.SSHHost = src.Host // gerçek kullanılan hedef (env dahil) raporlanır
	if accs, err := accounts.Load(src); err != nil {
		d.AccountsErr = err
	} else {
		d.AccountsOK = len(accs)
	}

	if pool != nil {
		for _, p := range pool.Providers {
			pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			nums, err := p.Numbers(pctx)
			cancel()
			if err != nil {
				d.ProvidersErr = append(d.ProvidersErr, p.Name()+": "+err.Error())
				continue
			}
			d.ProvidersOK = append(d.ProvidersOK, fmt.Sprintf("%s: %d numara", p.Name(), len(nums)))
		}
	}
	return d
}

// CheckKahinTools, çalışan bir Kahin istemcisinden tool setini doğrular.
func CheckKahinTools(c *kahin.Client) (kahin.Toolset, error) {
	tools, err := c.ListTools()
	if err != nil {
		return kahin.Toolset{}, err
	}
	return kahin.CheckTools(tools), nil
}

// Ensure browser liveness with a mutex to avoid duplicate starts.
var startMu sync.Mutex

// StartBrowser, Kahin motorunu başlatır (zaten çalışıyorsa yeniden kullanır).
func StartBrowser(c *kahin.Client, cfg kahin.RuntimeConfig) (*kahin.Browser, map[string]any, error) {
	startMu.Lock()
	defer startMu.Unlock()
	b := kahin.NewBrowser(c)
	info, err := b.Start(true, "keş", true)
	if err != nil {
		// Zaten çalışıyorsa motoru yeniden kullan.
		if h, herr := b.Health(); herr == nil {
			if alive, _ := h["alive"].(bool); alive {
				return b, h, nil
			}
		}
		return b, nil, err
	}
	return b, info, nil
}

// Package flow, uçtan uca kayıt akışını yönetir:
//
//	Gitee hesabı (laptop) → hedef sitede "Gitee ile kayıt ol" →
//	telefon numarası (receive-SMS) → SMS kodu → kayıt tamamla
//
// CAPTCHA çıkarsa captcha.Solver (Grok CLI) devreye girer.
//
// PARALEL ÇALIŞMA: hedefler (site × hesap × numara) eşzamanlı işlenir.
// Her işçi kendi Kahin tarayıcı slotunda (KAHIN_BROWSER_LOCK_PATH +
// KAHIN_HOME ayrı) çalışır; yarış durumu yoktur, her işçi bağımsızdır.
package flow

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/void0x14/china-sites-register/internal/accounts"
	"github.com/void0x14/china-sites-register/internal/captcha"
	"github.com/void0x14/china-sites-register/internal/kahin"
	"github.com/void0x14/china-sites-register/internal/sites"
	"github.com/void0x14/china-sites-register/internal/sms"
)

// Options, akış yapılandırmasıdır.
type Options struct {
	Accounts []accounts.Account
	// Sites, denenecek hedef siteler (birden çok site paralel).
	Sites []sites.Site
	// Pool, varsayılan SMS havuzu (PoolFactory yoksa kullanılır).
	Pool *sms.Pool
	// PoolFactory, işçi başına SMS havuzu üretir. Tarayıcı köprüsü gerektiren
	// sağlayıcılar (sms24, quackr) o işçinin tarayıcısına bağlanmalıdır;
	// bu yüzden havuz işçi başına kurulur. Nil ise Pool kullanılır.
	PoolFactory func(worker int, b *kahin.Browser) *sms.Pool
	// BrowserFactory, her işçi için ayrı Kahin tarayıcısı üretir.
	// Paralel işçi sayısı kadar çağrılır; her biri ayrı slot/kilit kullanır.
	BrowserFactory func(worker int) (*kahin.Browser, func(), error)
	// SolverFactory, her işçi için ayrı CAPTCHA çözücü üretir.
	SolverFactory func(worker int) *captcha.Solver

	PreferredCC string   // tercih edilen ülke kodu (ör. "86")
	AllowedCCs  []string // yalnızca bu ülke kodlarındaki numaralar kabul edilir
	CodeTimeout time.Duration

	// Workers, eşzamanlı işçi (tarayıcı) sayısı.
	Workers int
	// MaxPhoneAttempts, bir hedef için en fazla kaç farklı numara denenecek.
	MaxPhoneAttempts int
	// MaxAccountsPerSite, her siteye kaç hesap atanacak (0 = tümü).
	MaxAccountsPerSite int

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

// job, tek bir (site, hesap) denemesidir.
type job struct {
	site sites.Site
	acc  accounts.Account
}

// Run, tüm hedefleri EŞZAMANLI olarak işler.
//
// Her işçi kendi tarayıcısında bir iş alır; numaralar paylaşımlı havuzdan
// rastgele ve dışlamalı seçilir (aynı numara iki işçiye düşmez).
func Run(ctx context.Context, o Options) *Report {
	if o.Log == nil {
		o.Log = func(s string) { fmt.Fprintln(os.Stdout, s) }
	}
	if o.CodeTimeout <= 0 {
		o.CodeTimeout = 5 * time.Minute
	}
	if o.MaxPhoneAttempts <= 0 {
		o.MaxPhoneAttempts = 3
	}
	if o.Workers <= 0 {
		o.Workers = 1
	}
	rep := &Report{Started: time.Now()}
	defer func() { rep.Finished = time.Now() }()

	// İş kuyruğu: site × hesap.
	jobs := buildJobs(o)
	if len(jobs) == 0 {
		o.Log("! iş yok (hesap/site seçilmedi)")
		return rep
	}
	o.Log(fmt.Sprintf("== %d iş, %d eşzamanlı işçi, %d site ==", len(jobs), o.Workers, len(o.Sites)))

	var (
		jobCh  = make(chan job)
		resMu  sync.Mutex
		usedMu sync.Mutex
		used   = map[string]bool{} // paylaşımlı: dünyada kullanılmış numaralar
		done   atomic.Int64
		wg     sync.WaitGroup
	)

	// İşçiler.
	for w := 0; w < o.Workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			br, cleanup, err := o.BrowserFactory(worker)
			if err != nil {
				o.Log(fmt.Sprintf("[işçi %d] tarayıcı açılamadı: %v", worker, err))
				return
			}
			defer cleanup()
			var solver *captcha.Solver
			if o.SolverFactory != nil {
				solver = o.SolverFactory(worker)
			}
			for j := range jobCh {
				select {
				case <-ctx.Done():
					return
				default:
				}
				res := runJob(ctx, o, worker, br, solver, j, &usedMu, used)
				resMu.Lock()
				rep.Results = append(rep.Results, res)
				resMu.Unlock()
				n := done.Add(1)
				mark := "✗"
				if res.Success {
					mark = "✓"
				}
				o.Log(fmt.Sprintf("   %s [%d/%d] %s %s | %s | %s",
					mark, n, len(jobs), res.Site, res.Account.Username, res.Phone.E164, stageText(res)))
			}
		}(w)
	}

	// İş dağıt.
	go func() {
		defer close(jobCh)
		for _, j := range jobs {
			select {
			case <-ctx.Done():
				return
			case jobCh <- j:
			}
		}
	}()

	wg.Wait()
	return rep
}

func stageText(r *sites.Result) string {
	if r.Success {
		return "kayıt başarılı"
	}
	if r.Err != nil {
		return r.Stage + ": " + r.Err.Error()
	}
	return r.Stage
}

// buildJobs, site × hesap iş listesini kurar.
func buildJobs(o Options) []job {
	var out []job
	for _, s := range o.Sites {
		accs := o.Accounts
		if o.MaxAccountsPerSite > 0 && o.MaxAccountsPerSite < len(accs) {
			accs = accs[:o.MaxAccountsPerSite]
		}
		for _, a := range accs {
			out = append(out, job{site: s, acc: a})
		}
	}
	return out
}

// runJob, tek bir (site, hesap) için numara alıp kayıt akışını sürer.
//
// Numara, paylaşımlı "used" kümesinden dışlanarak seçilir; böylece iki işçi
// aynı numarayı kullanmaz (yarış yok). SMS kaynaklı başarısızlıkta farklı
// numara ile yeniden denenir.
func runJob(ctx context.Context, o Options, worker int, b *kahin.Browser, solver *captcha.Solver, j job, usedMu *sync.Mutex, used map[string]bool) *sites.Result {
	var last *sites.Result
	for attempt := 1; attempt <= o.MaxPhoneAttempts; attempt++ {
		select {
		case <-ctx.Done():
			if last != nil {
				return last
			}
			return &sites.Result{Site: j.site.Name(), Account: j.acc, Stage: "iptal", Err: ctx.Err()}
		default:
		}

		// Bu denemede kullanılacak numaranın dışlama kümesini al.
		usedMu.Lock()
		exclude := make(map[string]bool, len(used))
		for k := range used {
			exclude[k] = true
		}
		usedMu.Unlock()

		res := runOne(ctx, o, worker, b, solver, j.site, j.acc, exclude)

		// Numara kullanıldı olarak işaretle (başarılı/başarısız fark etmez).
		if res.Phone.E164 != "" {
			usedMu.Lock()
			used[res.Phone.E164] = true
			usedMu.Unlock()
		}
		last = res
		if res.Success {
			return res
		}
		if !retryableStage(res.Stage) {
			return res
		}
	}
	return last
}

// retryableStage, numara değiştirip yeniden denemenin anlamlı olduğu aşamayı söyler.
func retryableStage(stage string) bool {
	switch stage {
	case "gitcode_kayıt", "kayıt", "numara_alma":
		return true
	default:
		return false
	}
}

// codePrefer, paylaşımlı numaralarda hedef sitenin SMS'ini ayırt eder.
//
// free-sms-receive.com numaralarına saniyeler içinde başka servislerin
// SMS'leri düşer (canlı kanıt: 中华万年历, 考研帮, 招商银行...). gitcode'nin
// gönderdiği kod 中国联通/中国移动/中国电信 veya gitcode imzası taşır; bu
// yüzden önce bu imzalar aranır.
//
// DİKKAT: bu bir TERCİH'tir, zorunluluk değil. Eşleşme olmazsa grace süresi
// sonunda ilk kod yedek olarak kabul edilir (yanlış süzgeç akışı kilitlemesin).
func codePrefer(site sites.Site) func(sms.Message) bool {
	name := site.Name()
	return func(m sms.Message) bool {
		t := m.Text
		switch name {
		case "gitcode", "gitlink", "jihulab":
			for _, mark := range []string{"gitcode", "GitCode", "联通", "移动", "电信", "原子", "AtomGit"} {
				if strings.Contains(t, mark) {
					return true
				}
			}
		}
		return false
	}
}

func runOne(ctx context.Context, o Options, worker int, b *kahin.Browser, solver *captcha.Solver, site sites.Site, acc accounts.Account, exclude map[string]bool) *sites.Result {
	res := &sites.Result{Site: site.Name(), Account: acc}

	// SMS havuzu: işçi başına (tarayıcı köprüsü gerektiren sağlayıcılar o
	// işçinin tarayıcısına bağlanır).
	pool := o.Pool
	if o.PoolFactory != nil {
		if p := o.PoolFactory(worker, b); p != nil {
			pool = p
		}
	}

	// Numara: hedef formun kabul ettiği ülke kodlarından, dışlamalı.
	acq, err := pool.AcquireExcluding(ctx, o.PreferredCC, exclude, o.AllowedCCs...)
	if acq != nil {
		for _, a := range acq.Attempts {
			o.Log(fmt.Sprintf("   [işçi %d] sms: %s", worker, a))
		}
	}
	if err != nil {
		res.Stage = "numara_alma"
		res.Err = err
		return res
	}
	res.Phone = acq.Number

	// CAPTCHA çözücüyü adaptöre bağla (adaptör başına taze atama).
	if g, ok := site.(*sites.GitCode); ok {
		g.Solver = solver
	}
	if g, ok := site.(*sites.GitLink); ok {
		g.Solver = solver
	}
	if g, ok := site.(*sites.JiHuLab); ok {
		g.Solver = solver
	}

	// SMS kodu okuyucu: mevcut mesajları "görüldü" işaretle, sonra bekle.
	prov := findProvider(pool, acq.Provider)
	seen := map[string]bool{}
	seedSeen(ctx, prov, acq.Number, seen)
	codeFn := func(cctx context.Context, n sms.Number) (string, error) {
		// Paylaşımlı numaralarda başka servislerin SMS'leri düşer; hedef sitenin
		// kodunu tercih et (bkz. sms.WaitForCodePref). Tercih eşleşmezse grace
		// sonrası ilk kod yedek olarak kabul edilir.
		code, text, err := pool.WaitForCodePref(cctx, prov, n, seen, o.CodeTimeout, 8*time.Second, codePrefer(site), 90*time.Second)
		if err != nil {
			return "", err
		}
		o.Log(fmt.Sprintf("   [işçi %d] SMS: %s", worker, truncate(text, 120)))
		return code, nil
	}

	out, err := site.Register(ctx, b, acc, acq.Number, codeFn)
	if err != nil && out != nil {
		out.Err = err
	}
	if out == nil {
		out = &sites.Result{Site: site.Name(), Account: acc, Phone: acq.Number, Err: err, Stage: "bilinmeyen"}
	}
	return out
}

// ---------------------------------------------------------------------------
// yardımcılar
// ---------------------------------------------------------------------------

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

var startMu sync.Mutex

// StartBrowser, Kahin motorunu başlatır (zaten çalışıyorsa yeniden kullanır).
func StartBrowser(c *kahin.Client, cfg kahin.RuntimeConfig) (*kahin.Browser, map[string]any, error) {
	startMu.Lock()
	defer startMu.Unlock()
	b := kahin.NewBrowser(c)
	info, err := b.Start(true, "keş", true)
	if err != nil {
		if h, herr := b.Health(); herr == nil {
			if alive, _ := h["alive"].(bool); alive {
				return b, h, nil
			}
		}
		return b, nil, err
	}
	return b, info, nil
}

// chinareg — Çin git platformlarına (gitcode/gitlink/jihulab) Gitee hesabıyla
// otomatik kayıt.
//
// Akış:
//
//	laptop ~/gitee/hesaplar.txt  →  hedef sitede "Gitee ile kayıt ol" (OAuth)
//	→  receive-SMS'ten numara  →  SMS kodu  →  kayıt tamam
//
// CAPTCHA çıkarsa Grok CLI (workbuddy/global:deepseek-v4.1-flash) çözer.
//
// Kullanım:
//
//	chinareg doctor                      # ortam kontrolü (Kahin, Grok, SSH, SMS)
//	chinareg accounts                    # hesap havuzunu listele
//	chinareg sites                       # hedef site adaptörlerini listele
//	chinareg run -site gitcode -limit 1  # tek hesapla canlı kayıt
//	chinareg run -site gitcode -all      # tüm hesaplarla sırayla
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/void0x14/china-sites-register/internal/accounts"
	"github.com/void0x14/china-sites-register/internal/captcha"
	"github.com/void0x14/china-sites-register/internal/flow"
	"github.com/void0x14/china-sites-register/internal/kahin"
	"github.com/void0x14/china-sites-register/internal/sites"
	"github.com/void0x14/china-sites-register/internal/sms"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "doctor":
		err = cmdDoctor(args)
	case "accounts":
		err = cmdAccounts(args)
	case "sites":
		err = cmdSites(args)
	case "sms":
		err = cmdSMS(args)
	case "run":
		err = cmdRun(args)
	case "oauth":
		err = cmdOAuth(args)
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "bilinmeyen komut: %s\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hata: "+err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `chinareg — Çin git platformlarına Gitee ile otomatik kayıt

Komutlar:
  doctor     Ortam kontrolü: Kahin, Grok CLI, SSH hesap havuzu, SMS sağlayıcıları
  accounts   Laptop'taki hesap havuzunu listele
  sites      Hedef site adaptörlerini listele
  sms        Receive-SMS sağlayıcılarından canlı numara listesi
  oauth      Gitee OAuth ("Gitee ile kayıt ol") akışını canlı sür
  run        Uçtan uca kayıt akışını çalıştır

Ortam değişkenleri:
  SSH_LAPTOP_HOST      laptop SSH hedefi (ör. void0x14@192.168.1.28)
  SSH_LAPTOP_GITEE_DIR hesap dizini (varsayılan ~/gitee)
  SSH_LAPTOP_KEY       SSH özel anahtar yolu
  KAHIN_PYTHON         Kahin python yorumlayıcısı
  KAHIN_DIR            Kahin paket kökü
  KAHIN_BROWSER_LOCK_PATH  Ayrı tarayıcı slotu (paralel kullanım)
  KAHIN_HOME           Ayrı profil/kalıcı veri kökü
  GROK_BIN             grok CLI yolu
  GROK_MODEL           CAPTCHA modeli (varsayılan workbuddy/global:deepseek-v4.1-flash)
`)
}

// ---------------------------------------------------------------------------
// doctor
// ---------------------------------------------------------------------------

func cmdDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	live := fs.Bool("live", false, "Kahin motorunu gerçekten başlat ve tool setini doğrula")
	_ = fs.Parse(args)

	ctx, cancel := signalCtx()
	defer cancel()

	solver := captcha.Default()
	pool := defaultPool()

	fmt.Println("== chinareg doctor ==")
	fmt.Printf("grok      : %s | model=%s\n", solver.Bin, solver.Model)
	if _, err := os.Stat(solver.Bin); err != nil {
		fmt.Printf("            ⚠ grok bulunamadı: %v\n", err)
	}

	d := flow.Diagnose(ctx, "", pool, solver)

	fmt.Printf("kahin     : python=%s\n", d.KahinConfig.Python)
	fmt.Printf("            dir=%s\n", d.KahinConfig.Dir)
	if d.KahinConfig.Lock != "" {
		fmt.Printf("            lock=%s\n", d.KahinConfig.Lock)
	}
	if d.KahinConfig.Home != "" {
		fmt.Printf("            home=%s\n", d.KahinConfig.Home)
	}

	fmt.Printf("ssh       : %s\n", nonEmpty(d.SSHHost, "(SSH_LAPTOP_HOST ayarlı değil)"))
	if d.AccountsErr != nil {
		fmt.Printf("hesaplar  : ✗ %v\n", d.AccountsErr)
	} else {
		fmt.Printf("hesaplar  : ✓ %d hesap\n", d.AccountsOK)
	}

	fmt.Println("sms       :")
	for _, ok := range d.ProvidersOK {
		fmt.Printf("            ✓ %s\n", ok)
	}
	for _, bad := range d.ProvidersErr {
		fmt.Printf("            ✗ %s\n", bad)
	}
	for _, n := range d.Notes {
		fmt.Printf("not       : %s\n", n)
	}

	if *live {
		c, err := kahin.NewClient(d.KahinConfig.Options())
		if err != nil {
			return fmt.Errorf("kahin başlatma: %w", err)
		}
		defer c.Close()
		ts, err := flow.CheckKahinTools(c)
		if err != nil {
			return err
		}
		fmt.Printf("kahin tool: %d gerekli, %d eksik\n", len(ts.Required), len(ts.Missing))
		if len(ts.Missing) > 0 {
			fmt.Printf("            ✗ eksik: %s\n", strings.Join(ts.Missing, ", "))
		}
		b, _, err := flow.StartBrowser(c, d.KahinConfig)
		if err != nil {
			fmt.Printf("kahin motor: ✗ %v\n", err)
		} else {
			h, _ := b.Health()
			fmt.Printf("kahin motor: ✓ %v\n", h)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// accounts
// ---------------------------------------------------------------------------

func cmdAccounts(args []string) error {
	fs := flag.NewFlagSet("accounts", flag.ExitOnError)
	showPass := fs.Bool("show-secrets", false, "şifreleri ve PAT'leri göster")
	_ = fs.Parse(args)

	accs, err := loadAccounts()
	if err != nil {
		return err
	}
	fmt.Printf("%d hesap\n", len(accs))
	for i, a := range accs {
		if *showPass {
			fmt.Printf("%3d  %-28s  %-10s  pass=%s  pat=%s\n", i+1, a.Email, a.Username, a.Password, mask(a.PAT))
		} else {
			fmt.Printf("%3d  %-28s  %-10s  pat=%s\n", i+1, a.Email, a.Username, mask(a.PAT))
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// sites
// ---------------------------------------------------------------------------

func cmdSites(args []string) error {
	for _, s := range sites.All() {
		fmt.Printf("%-10s  oauth=%s\n", s.Name(), s.OAuthLoginURL())
		fmt.Printf("           desteklenen ülke kodları: %s\n", strings.Join(s.SupportedCCs(), ","))
	}
	return nil
}

// ---------------------------------------------------------------------------
// sms
// ---------------------------------------------------------------------------

func cmdSMS(args []string) error {
	fs := flag.NewFlagSet("sms", flag.ExitOnError)
	limit := fs.Int("limit", 15, "gösterilecek numara sayısı")
	cc := fs.String("cc", "", "tercih edilen ülke kodu (ör. 86)")
	watch := fs.Bool("watch", false, "seçilen numaranın mesajlarını izle")
	_ = fs.Parse(args)

	ctx, cancel := signalCtx()
	defer cancel()

	pool := defaultPool()
	acq, err := pool.Acquire(ctx, *cc)
	for _, a := range acq.Attempts {
		fmt.Println("  " + a)
	}
	if err != nil {
		return err
	}
	fmt.Printf("seçilen: %s (%s)\n", acq.Number.E164, acq.Provider)
	prov := providerByName(pool, acq.Provider)
	nums, _ := prov.Numbers(ctx)
	if len(nums) > *limit {
		nums = nums[:*limit]
	}
	for i, n := range nums {
		fmt.Printf("%3d  %s  cc=%s  country=%s\n", i+1, n.E164, n.CC, n.Country)
	}

	if *watch {
		fmt.Println("-- mesajlar izleniyor (Ctrl-C ile çık) --")
		seen := map[string]bool{}
		deadline := time.Now().Add(10 * time.Minute)
		for time.Now().Before(deadline) {
			msgs, err := prov.Messages(ctx, acq.Number)
			if err == nil {
				for _, m := range msgs {
					key := m.From + "|" + m.Text
					if seen[key] {
						continue
					}
					seen[key] = true
					fmt.Printf("  [%s] %s\n", m.From, m.Text)
					if code := sms.ExtractCode(m.Text); code != "" {
						fmt.Printf("  → kod: %s\n", code)
					}
				}
			}
			time.Sleep(8 * time.Second)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// oauth
// ---------------------------------------------------------------------------

func cmdOAuth(args []string) error {
	fs := flag.NewFlagSet("oauth", flag.ExitOnError)
	siteName := fs.String("site", "gitcode", "hedef site")
	limit := fs.Int("limit", 1, "kaç hesap denenecek")
	from := fs.Int("from", 0, "başlanacak hesap indeksi (0 tabanlı)")
	noLogin := fs.Bool("no-login", false, "Gitee girişini atla (mevcut oturumu kullan)")
	_ = fs.Parse(args)

	ctx, cancel := signalCtx()
	defer cancel()

	site, err := siteByName(*siteName)
	if err != nil {
		return err
	}
	accs, err := loadAccounts()
	if err != nil {
		return err
	}
	if *from > 0 && *from < len(accs) {
		accs = accs[*from:]
	}
	if *limit > 0 && *limit < len(accs) {
		accs = accs[:*limit]
	}

	cfg, err := kahin.Resolve()
	if err != nil {
		return err
	}
	c, err := kahin.NewClient(cfg.Options())
	if err != nil {
		return err
	}
	defer c.Close()
	ts, err := flow.CheckKahinTools(c)
	if err != nil {
		return err
	}
	if len(ts.Missing) > 0 {
		return fmt.Errorf("kahin tool eksik: %s", strings.Join(ts.Missing, ", "))
	}
	b, _, err := flow.StartBrowser(c, cfg)
	if err != nil {
		return err
	}

	gc, ok := site.(*sites.GitCode)
	if !ok {
		return fmt.Errorf("oauth: site GitCode değil")
	}
	gc.Solver = captcha.Default()

	for i, acc := range accs {
		fmt.Printf("== [%d/%d] %s (%s)\n", i+1, len(accs), acc.Username, acc.Email)
		if *noLogin {
			if err := b.Navigate(gc.OAuthLoginURL(), "domcontentloaded", 45*time.Second); err != nil {
				fmt.Printf("   ✗ oauth başlatma: %v\n", err)
				continue
			}
			info, _ := b.Info()
			fmt.Printf("   url: %s | %s\n", info.URL, info.Title)
			continue
		}
		if err := gc.GiteeLogin(b, acc); err != nil {
			var capErr *sites.CaptchaRequiredError
			if errors.As(err, &capErr) {
				fmt.Printf("   ⚠ CAPTCHA (%s), Grok CLI çözüyor...\n", capErr.Kind)
				if serr := sites.SolveGiteeSlider(ctx, b, gc.Solver, ""); serr != nil {
					fmt.Printf("   ✗ CAPTCHA çözülemedi: %v\n", serr)
					continue
				}
				if !sites.WaitLogin(b, 30*time.Second) {
					fmt.Printf("   ✗ CAPTCHA sonrası oturum kurulmadı\n")
					continue
				}
			} else {
				fmt.Printf("   ✗ gitee giriş: %v\n", err)
				continue
			}
		}
		info, _ := b.Info()
		fmt.Printf("   ✓ giriş: %s | %s\n", info.URL, info.Title)

		if err := gc.OAuthAuthorize(ctx, b); err != nil {
			fmt.Printf("   ✗ oauth: %v\n", err)
			continue
		}
		info, _ = b.Info()
		fmt.Printf("   son: %s | %s\n", info.URL, info.Title)
		if strings.Contains(info.Body, "Set username") {
			fmt.Println("   ✓ KAYIT FORMU GÖRÜNDÜ (telefon + SMS kodu bekleniyor)")
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// run
// ---------------------------------------------------------------------------

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	siteName := fs.String("site", "all", "hedef site: all | gitcode | gitlink | jihulab")
	limit := fs.Int("limit", 1, "site başına kaç hesap denenecek")
	all := fs.Bool("all", false, "tüm hesaplarla dene")
	cc := fs.String("cc", "", "tercih edilen ülke kodu (ör. 86)")
	codeTimeout := fs.Duration("code-timeout", 5*time.Minute, "SMS kodu bekleme süresi")
	phoneAttempts := fs.Int("phone-attempts", 3, "hesap başına denenecek farklı numara sayısı")
	from := fs.Int("from", 0, "başlanacak hesap indeksi (0 tabanlı)")
	workers := fs.Int("workers", 4, "eşzamanlı tarayıcı/işçi sayısı")
	_ = fs.Parse(args)

	ctx, cancel := signalCtx()
	defer cancel()

	// Site seçimi: "all" → tüm siteler eşzamanlı.
	var chosen []sites.Site
	if strings.EqualFold(*siteName, "all") || *siteName == "" {
		chosen = sites.All()
	} else {
		s, err := siteByName(*siteName)
		if err != nil {
			return err
		}
		chosen = []sites.Site{s}
	}

	accs, err := loadAccounts()
	if err != nil {
		return err
	}
	if *from > 0 && *from < len(accs) {
		accs = accs[*from:]
	}
	if !*all && *limit > 0 && *limit < len(accs) {
		accs = accs[:*limit]
	}

	cfg, err := kahin.Resolve()
	if err != nil {
		return err
	}

	// Her işçi KENDİ Kahin sürecini ve KENDİ tarayıcı slotunu kullanır:
	// ayrı KAHIN_BROWSER_LOCK_PATH + KAHIN_HOME verilmezse makine geneli tek
	// slot çakışır (engine_process_conflict). Bu yüzden işçi başına ayrı
	// kilit/profil yolu üretilir.
	//
	// KAHIN_BROWSER_LOCK_PATH verilmişse onun DİZİNİ taban alınır: çağıran
	// kendi izole slotunu (ör. /tmp/kahin-loop) seçtiğinde işçi yolları da o
	// slotun içinde kalır. Sabit /tmp/kahin-worker-N kullanılırsa aynı anda
	// koşan iki chinareg süreci aynı kilit dosyasına çakışır ve ikincisi
	// engine_process_conflict alır (canlı kanıt: eşzamanlı fire).
	base := os.TempDir()
	if d := os.Getenv("KAHIN_BROWSER_LOCK_PATH"); d != "" {
		base = filepath.Dir(d)
	}
	browserFactory := func(worker int) (*kahin.Browser, func(), error) {
		wcfg := cfg
		wcfg.Lock = filepath.Join(base, fmt.Sprintf("kahin-worker-%d/browser.lock", worker))
		wcfg.Home = filepath.Join(base, fmt.Sprintf("kahin-worker-%d/home", worker))
		wcfg.Profile = filepath.Join(base, fmt.Sprintf("kahin-worker-%d/profile", worker))
		if err := os.MkdirAll(filepath.Dir(wcfg.Lock), 0o755); err != nil {
			return nil, nil, err
		}
		if err := os.MkdirAll(wcfg.Home, 0o755); err != nil {
			return nil, nil, err
		}
		c, err := kahin.NewClient(wcfg.Options())
		if err != nil {
			return nil, nil, fmt.Errorf("işçi %d kahin istemcisi: %w", worker, err)
		}
		ts, err := flow.CheckKahinTools(c)
		if err != nil {
			c.Close()
			return nil, nil, err
		}
		if len(ts.Missing) > 0 {
			c.Close()
			return nil, nil, fmt.Errorf("işçi %d kahin tool eksik: %s", worker, strings.Join(ts.Missing, ", "))
		}
		b, _, err := flow.StartBrowser(c, wcfg)
		if err != nil {
			c.Close()
			return nil, nil, fmt.Errorf("işçi %d tarayıcı: %w", worker, err)
		}
		return b, func() { c.Close() }, nil
	}

	solverFactory := func(worker int) *captcha.Solver { return captcha.Default() }
	pool := defaultPool()

	rep := flow.Run(ctx, flow.Options{
		Accounts:           accs,
		Sites:              chosen,
		Pool:               pool,
		BrowserFactory:     browserFactory,
		SolverFactory:      solverFactory,
		PreferredCC:        *cc,
		AllowedCCs:         allSupportedCCs(chosen),
		CodeTimeout:        *codeTimeout,
		Workers:            *workers,
		MaxPhoneAttempts:   *phoneAttempts,
		MaxAccountsPerSite: pickLimit(*all, *limit),
		Log:                func(s string) { fmt.Println(s) },
	})

	fmt.Printf("\n== özet: %d/%d başarılı (%s) ==\n",
		rep.OK(), len(rep.Results), rep.Finished.Sub(rep.Started).Round(time.Second))
	for _, r := range rep.Results {
		status := "✗ " + r.Stage
		if r.Success {
			status = "✓"
		}
		fmt.Printf("  %-10s %-10s %-6s %s\n", r.Site, r.Account.Username, status, r.Phone.E164)
	}
	if rep.OK() == 0 {
		return fmt.Errorf("hiç kayıt başarılı olmadı")
	}
	return nil
}

// allSupportedCCs, seçilen sitelerin kabul ettiği ülke kodlarının kesişimidir.
//
// Birden çok site paralel deneneceğinde, bir numaranın TÜM sitelerde
// kullanılabilmesi için kesişim alınır.
func allSupportedCCs(ss []sites.Site) []string {
	if len(ss) == 0 {
		return nil
	}
	inter := map[string]bool{}
	for _, cc := range ss[0].SupportedCCs() {
		inter[cc] = true
	}
	for _, s := range ss[1:] {
		next := map[string]bool{}
		for _, cc := range s.SupportedCCs() {
			if inter[cc] {
				next[cc] = true
			}
		}
		inter = next
	}
	var out []string
	for cc := range inter {
		out = append(out, cc)
	}
	sort.Strings(out)
	return out
}

func pickLimit(all bool, limit int) int {
	if all {
		return 0
	}
	return limit
}

// ---------------------------------------------------------------------------
// ortak
// ---------------------------------------------------------------------------

func siteByName(name string) (sites.Site, error) {
	for _, s := range sites.All() {
		if strings.EqualFold(s.Name(), name) {
			return s, nil
		}
	}
	return nil, fmt.Errorf("bilinmeyen site: %s (mevcut: %s)", name, strings.Join(sites.Names(), ", "))
}

// defaultPool, canlı doğrulanmış sağlayıcılarla havuz kurar.
//
// ÖNCELİK: gitcode SMS ucu YALNIZCA +86 kabul eder (canlı kanıt). +86 veren
// iki kaynak var:
//
//  1. free-sms-receive.com — numara listesi + mesajlar DÜZ HTTP ile okunur
//     (canlı kanıt: 5 numara, numara başına 15 mesaj, kod regex'i eşleşiyor).
//     Bu yüzden kod bekleyebilen TEK +86 kaynağı budur; ilk sıradadır.
//  2. quackr — +86 numara verir ama mesaj okuma Cloudflare Turnstile ister ve
//     tarayıcı köprüsü (QuackrBrowser) bağlı değildir; kod okunamaz.
//     Numara kaynağı olarak yedek kalır.
//
// freephonenum ve receive-sms-free.cc +86 vermez (uluslararası); son çare.
func defaultPool() *sms.Pool {
	return sms.NewPool(
		&sms.FreeSMSReceive{Client: sms.NewHTTPClient()},
		&sms.Quackr{Client: sms.NewHTTPClient()},
		&sms.FreePhoneNum{Client: sms.NewHTTPClient(), Countries: []string{"us", "gb", "ca", "pl", "se"}},
		&sms.ReceiveSMSFreeCC{Client: sms.NewHTTPClient(), CountrySlugs: []string{"USA", "UK", "Sweden", "Finland"}},
	)
}

func providerByName(p *sms.Pool, name string) sms.Provider {
	for _, prov := range p.Providers {
		if prov.Name() == name {
			return prov
		}
	}
	return p.Providers[0]
}

func loadAccounts() ([]accounts.Account, error) {
	return accounts.Load(accounts.DefaultSSHSource())
}

func signalCtx() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		fmt.Println("\n! kesme sinyali, kapatılıyor...")
		cancel()
	}()
	return ctx, cancel
}

func mask(s string) string {
	if s == "" {
		return "-"
	}
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:8] + "…"
}

func nonEmpty(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

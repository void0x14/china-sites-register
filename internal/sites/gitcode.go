// Package sites, hedef Çin git platformlarındaki "Gitee ile kayıt ol" (OAuth)
// akışını sürer.
//
// CANLI DOĞRULANAN AKIŞ (gitcode.com / AtomGit):
//
//  1. https://gitcode.com/uc/api/v1/oauth/login/gitee  → 302
//     https://gitee.com/oauth/authorize?response_type=code
//     &client_id=a3c734c5166b090c2e1f3efc6d94452a96fa898b99eeccb53bec2e547f819e19
//     &redirect_uri=https://gitcode.com/oauth/callback?type=gitee&domain=web-api.gitcode.com
//     &scope=user_info
//
//  2. Giriş yapılmamışsa gitee.com/login'e düşer. Form:
//     action="/login", POST
//     user[login]    = e-posta / kullanıcı adı
//     user[password] = BOŞ kalır
//     encrypt_data[user[password]] = ŞİFRE DÜZ METİN olarak buraya yazılır
//     (+ authenticity_token, utf8, encrypt_key="password")
//     Yanlış alan kullanılırsa "Invalid email or password." döner.
//
//  3. Giriş başarılıysa gitee.com/oauth/authorize sayfası gelir:
//     "OAuth 授权请求 <kullanıcı> | 换个帐号?" ve onay formu (POST /oauth/authorize)
//     submit: input[value=Permmit]  (ret: input[value=Deny])
//
//  4. Callback: https://gitcode.com/oauth/callback?type=gitee&...&code=...
//     Sayfa: "第三方授权 - AtomGit" — KAYIT FORMU:
//     - Set username            (Gitee kullanıcı adı ön dolu gelir)
//     - Verify phone number     (+86 seçici; ülke listesi: 86,852,886,1,7,33,...)
//     - Verification code  + "Get verification code"
//     - [ ] User Agreement + Privacy Policy
//     - [ ] share my account ... with AtomGit
//     - button "Create account and continue" / "Cancel"
package sites

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/void0x14/china-sites-register/internal/accounts"
	"github.com/void0x14/china-sites-register/internal/captcha"
	"github.com/void0x14/china-sites-register/internal/kahin"
	"github.com/void0x14/china-sites-register/internal/sms"
)

// Result, bir kayıt denemesinin sonucudur.
type Result struct {
	Site     string
	Account  accounts.Account
	Phone    sms.Number
	Success  bool
	Stage    string // ulaşılan son aşama
	Evidence string // son sayfa url/başlık kanıtı
	Err      error
}

// Site, hedef platform adaptörüdür.
type Site interface {
	Name() string
	// OAuthLoginURL, "Gitee ile kayıt ol" girişinin başlangıç URL'sidir.
	OAuthLoginURL() string
	// SupportedCCs, kayıt formunun kabul ettiği ülke kodlarıdır.
	// Numara seçimi bu kümeye göre yapılmalıdır; boşsa kısıt yoktur.
	SupportedCCs() []string
	// Register, hesap + numara ile kayıt akışını sürer.
	Register(ctx context.Context, b *kahin.Browser, acc accounts.Account, phone sms.Number, codeFn func(context.Context, sms.Number) (string, error)) (*Result, error)
}

// ---------------------------------------------------------------------------
// GitCode (AtomGit)
// ---------------------------------------------------------------------------

// GitCode, gitcode.com adaptörüdür.
type GitCode struct {
	// LoginTimeout, gitee girişinden sonra yönlendirme bekleme süresi.
	LoginTimeout time.Duration
	// Solver, Gitee girişinde çıkan CAPTCHA'yı çözer (Grok CLI).
	// nil ise CAPTCHA'ya girilmez ve hata döner.
	Solver *captcha.Solver
}

func (s *GitCode) Name() string { return "gitcode" }

// SupportedCCs, gitcode kayıt formunun ülke kodu seçicisinde bulunan
// ülke kodlarıdır (canlı okundu). Numara seçimi bu kümeye göre yapılmalıdır;
// aksi halde numara formda girilemez.
func (s *GitCode) SupportedCCs() []string {
	return []string{
		"86", "852", "886", "1", "7", "33", "351", "353", "358", "39",
		"41", "44", "46", "47", "48", "49", "54", "60", "65", "66",
		"90", "91", "92", "972",
	}
}

// OAuthLoginURL, gitcode'un Gitee OAuth başlangıcıdır.
// Bu uç nokta canlı doğrulandı: 302 → gitee.com/oauth/authorize.
func (s *GitCode) OAuthLoginURL() string {
	return "https://gitcode.com/uc/api/v1/oauth/login/gitee"
}

func (s *GitCode) Register(ctx context.Context, b *kahin.Browser, acc accounts.Account, phone sms.Number, codeFn func(context.Context, sms.Number) (string, error)) (*Result, error) {
	r := &Result{Site: s.Name(), Account: acc, Phone: phone}

	// --- Aşama 1: Gitee oturumu ---
	//
	// KRİTİK: OAuth URL'sine doğrudan gidip giriş yapmak İngilizce (oversea)
	// akışa düşer ve yabancı IP'de WAF tarafından sessizce reddedilir. Bu
	// yüzden önce çalışan zh-CN klasik akışla oturum kurulur; oturum çerezi
	// kalıcı olduğundan OAuth sonrası tekrar giriş gerekmez.
	if err := s.GiteeLogin(b, acc); err != nil {
		var capErr *CaptchaRequiredError
		if errors.As(err, &capErr) {
			// CAPTCHA çıktı: Grok CLI ile çöz, girişi sürdür.
			if serr := SolveGiteeSlider(ctx, b, s.Solver, rURL(b)); serr != nil {
				r.Stage = "gitee_captcha"
				r.Err = fmt.Errorf("%v; çözüm başarısız: %w", err, serr)
				return r, r.Err
			}
			// Çözümden sonra giriş tamamlanmış olmalı.
			if waitFor(b, 30*time.Second, func() bool { return loginSucceeded(b) }) != nil {
				r.Stage = "gitee_giriş"
				r.Err = fmt.Errorf("CAPTCHA çözüldü ama oturum kurulmadı")
				return r, r.Err
			}
		} else {
			r.Stage = "gitee_giriş"
			r.Err = err
			return r, err
		}
	}
	info, _ := b.Info()
	r.Evidence = info.URL + " | " + info.Title

	// --- Aşama 2: OAuth başlat + onay ---
	if err := s.OAuthAuthorize(ctx, b); err != nil {
		r.Stage = "gitee_onay"
		r.Err = err
		return r, err
	}
	info, _ = b.Info()
	r.Evidence = info.URL + " | " + info.Title

	// --- Aşama 3: gitcode kayıt formu ---
	if err := s.gitcodeRegister(ctx, b, acc, phone, codeFn); err != nil {
		r.Stage = "gitcode_kayıt"
		r.Err = err
		return r, err
	}
	r.Success = true
	r.Stage = "tamamlandı"
	info, _ = b.Info()
	r.Evidence = info.URL + " | " + info.Title
	return r, nil
}

// OAuthAuthorize, OAuth başlangıcına gidip onay ekranını geçer.
//
// CANLI DOĞRULANAN MEKANİK: onaydan sonra gitcode callback'e döner ve
// "第三方授权" (üçüncü taraf yetkilendirme) KAYIT FORMU gelir. Bazı
// durumlarda callback ana sayfaya yönlenir (kod tüketilmiş); bu durumda
// OAuth başlangıcı tekrar denenir — oturum zaten yetkili olduğu için
// doğrudan forma düşer.
func (s *GitCode) OAuthAuthorize(ctx context.Context, b *kahin.Browser) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := b.Navigate(s.OAuthLoginURL(), "domcontentloaded", 45*time.Second); err != nil {
			lastErr = fmt.Errorf("oauth başlatma: %w", err)
			continue
		}
		if err := s.GiteeAuthorize(ctx, b); err != nil {
			lastErr = err
			continue
		}
		// Kayıt formu göründüyse bitti.
		if evalBool(b, `(document.body?document.body.innerText:'').indexOf('Set username')>=0`) {
			return nil
		}
		// Callback'teyiz ama form henüz yok: kısa bekle.
		if waitFor(b, 20*time.Second, func() bool {
			return evalBool(b, `(document.body?document.body.innerText:'').indexOf('Set username')>=0`)
		}) == nil {
			return nil
		}
		lastErr = fmt.Errorf("kayıt formu gelmedi (url: %s)", rURL(b))
		time.Sleep(2 * time.Second)
	}
	return lastErr
}

// GiteeLogin, Gitee giriş akışını sürer.
//
// CANLI DOĞRULANAN MEKANİK (kritik, bu oturumda kanıtlandı):
//
//   - Gitee iki ayrı giriş akışı sunar:
//     1) oversea (İngilizce, varsayılan): .js-email-input → .js-continue-email
//     → .js-password-input → #oversea-login-form. YABANCI IP'DE BU AKIŞ
//     SESSİZCE REDDEDİLİR: POST, uygulamaya hiç varmadan WAF tarafından
//     kesilir ve /login?tox_token=... adresine döner (oturum kurulmaz,
//     CAPTCHA da gösterilmez). tox_token = Baidu WAF dinamik imzası.
//     2) classic (zh-CN, https://gitee.com/login#lang=zh-CN): #user_login +
//     #user_password → #new_user submit. BU AKIŞ ÇALIŞIR.
//
//   - Şifreleme: form data-encrypt="true" taşır; şifre sayfa JS'i tarafından
//     gizli encrypt_data[user[password]] alanına şifrelenir (172 karakter
//     base64). Bu şifreleme YALNIZCA gerçek input/change olaylarıyla
//     tetiklenir; dom_action "type" olay tetiklemez → encrypt_data boş kalır
//     → "Invalid email or password". Bu yüzden fillJS kullanılır.
//
//   - CAPTCHA KOŞULU (canlı kanıt): Gitee captcha'yı YALNIZCA hesabın
//     failed_count>2 olduğunda zorunlu kılar. /check_user_login POST'u
//     {"result":1,"failed_count":N} döndürür. failed_count hesap-globaldir
//     (oturumdan bağımsız artar). Taze hesapta (failed_count=0) captcha
//     ÇIKMAZ ve giriş doğrudan başarılı olur — canlı doğrulandı.
//
//   - Başarı kanıtı: URL gitee.com'a döner ve başlık "工作台" (iş masası) olur.
func (s *GitCode) GiteeLogin(b *kahin.Browser, acc accounts.Account) error {
	// Çalışan akış: zh-CN klasik form. Gitee varsayılan olarak İngilizce
	// (oversea) formu sunar; yabancı IP'de o akış WAF tarafından sessizce
	// reddedilir. Bu yüzden önce zh-CN'e geçilir, sonra klasik form beklenir.
	if err := b.Navigate("https://gitee.com/login#lang=zh-CN", "domcontentloaded", 45*time.Second); err != nil {
		return fmt.Errorf("gitee giriş sayfası: %w", err)
	}
	if err := ensureClassicForm(b); err != nil {
		return err
	}

	// Captcha koşulunu önceden öğren (hesap-global failed_count).
	//
	// NOT: Bu yalnızca teşhis/günlük amaçlıdır; giriş kararı captcha duvarının
	// gerçekten görünmesine göre verilir. failed_count>2 iken captcha
	// neredeyse kesin çıkar; taze hesapta (0) çıkmaz — canlı doğrulandı.
	if n, err := checkFailedCount(b, acc.Email); err == nil {
		if n > 2 {
			logf("   gitee: failed_count=%d (captcha bekleniyor)", n)
		}
	}

	if err := fillLogin(b, acc); err != nil {
		return err
	}
	if err := submitLogin(b); err != nil {
		return err
	}

	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if loginSucceeded(b) {
			return nil
		}
		if ok, kind := captchaWall(b); ok {
			return &CaptchaRequiredError{Kind: kind, Account: acc.Username}
		}
		if loginRejected(b) {
			return fmt.Errorf("gitee girişi reddedildi (hesap %s)", acc.Username)
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("gitee girişi zaman aşımı (hesap %s)", acc.Username)
}

// logf, teşhis satırı yazar (günlükleyici yoksa stderr).
var logf = func(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// ensureClassicForm, klasik (zh-CN) giriş formunun görünmesini sağlar.
//
// CANLI DOĞRULANAN MEKANİK: gitee.com/login İngilizce (oversea) açılır;
// #lang=zh-CN hash'i bazen JS tarafından geç uygulanır. Form gelmezse
// hash yeniden set edilip hashchange tetiklenir, ardından dil bağlantısı
// tıklanır.
func ensureClassicForm(b *kahin.Browser) error {
	if waitFor(b, 12*time.Second, func() bool {
		return evalBool(b, `!!document.querySelector('#user_login')`)
	}) == nil {
		return nil
	}
	// Hash'i yeniden uygula + hashchange tetikle.
	_, _ = b.EvalString(`(function(){
		if(location.hash!=='#lang=zh-CN'){location.hash='#lang=zh-CN'}
		window.dispatchEvent(new HashChangeEvent('hashchange'));
		return 'ok';
	})()`)
	if waitFor(b, 8*time.Second, func() bool {
		return evalBool(b, `!!document.querySelector('#user_login')`)
	}) == nil {
		return nil
	}
	// Dil bağlantısına gerçek fareyle tıkla.
	if pos, err := elementCenter(b, `a[href="#lang=zh-CN"]`); err == nil {
		_ = b.MouseClick(pos[0], pos[1])
	}
	if waitFor(b, 15*time.Second, func() bool {
		return evalBool(b, `!!document.querySelector('#user_login')`)
	}) == nil {
		return nil
	}
	// Son çare: zh-CN'e doğrudan git.
	if err := b.Navigate("https://gitee.com/login#lang=zh-CN", "domcontentloaded", 45*time.Second); err != nil {
		return fmt.Errorf("zh-CN giriş sayfası: %w", err)
	}
	if waitFor(b, 15*time.Second, func() bool {
		return evalBool(b, `!!document.querySelector('#user_login')`)
	}) != nil {
		return fmt.Errorf("gitee klasik giriş formu gelmedi")
	}
	return nil
}

// CaptchaRequiredError, girişin çözülebilir bir CAPTCHA duvarıyla
// durduğunu bildirir; çağıran katman çözücüyü devreye sokar.
type CaptchaRequiredError struct {
	Kind    string
	Account string
}

func (e *CaptchaRequiredError) Error() string {
	return fmt.Sprintf("gitee girişi CAPTCHA ile bloklandı (%s, hesap %s)", e.Kind, e.Account)
}

// fillLogin, kullanıcı adı ve şifreyi klasik forma yazar (fillJS ile).
func fillLogin(b *kahin.Browser, acc accounts.Account) error {
	login := acc.Email
	if login == "" {
		login = acc.Username
	}
	if _, err := b.EvalString(fillJS("#user_login", login)); err != nil {
		return fmt.Errorf("kullanıcı adı yazılamadı: %w", err)
	}
	if _, err := b.EvalString(fillJS("#user_password", acc.Password)); err != nil {
		return fmt.Errorf("şifre yazılamadı: %w", err)
	}
	// Şifrelemenin gerçekten üretildiğini doğrula (boşsa giriş kesin reddedilir).
	enc, err := b.EvalString(`(function(){var e=document.querySelector('input[name="encrypt_data[user[password]]"]');return e?String(e.value.length):'-1'})()`)
	if err != nil {
		return fmt.Errorf("şifreleme alanı okunamadı: %w", err)
	}
	if strings.Trim(enc, `"`) == "0" || strings.Trim(enc, `"`) == "-1" {
		return fmt.Errorf("şifre şifrelenmedi (encrypt_data boş); fillJS olay üretmedi")
	}
	return nil
}

// submitLogin, klasik giriş formunu gönderir.
func submitLogin(b *kahin.Browser) error {
	_, err := b.EvalString(`(function(){
		var f=document.querySelector('form#new_user');
		if(!f)return 'yok';
		var btn=f.querySelector('input[type=submit],button[type=submit]');
		if(btn){btn.click();return 'ok'}
		if(f.requestSubmit){f.requestSubmit();return 'ok'}
		return 'yok';
	})()`)
	if err != nil {
		return fmt.Errorf("klasik giriş gönderilemedi: %w", err)
	}
	return nil
}

// checkFailedCount, hesabın başarısız giriş sayısını /check_user_login'den okur.
//
// Canlı yanıt: {"result":1,"failed_count":N}. result=1 hesap var demektir.
func checkFailedCount(b *kahin.Browser, login string) (int, error) {
	out, err := b.EvalString(fmt.Sprintf(`(function(){
		var x=new XMLHttpRequest();
		x.open('POST','/check_user_login',false);
		x.setRequestHeader('Content-Type','application/x-www-form-urlencoded');
		x.send('user_login='+encodeURIComponent(%q));
		try{return JSON.stringify({s:x.status,b:x.responseText})}catch(e){return 'ERR'}
	})()`, login))
	if err != nil {
		return 0, err
	}
	out = strings.Trim(out, `"`)
	var res struct {
		Result      int `json:"result"`
		FailedCount int `json:"failed_count"`
	}
	// Gövde JSON metni olarak gömülü; dış zarfı çöz.
	var wrap struct {
		S int    `json:"s"`
		B string `json:"b"`
	}
	if err := json.Unmarshal([]byte(out), &wrap); err != nil {
		return 0, err
	}
	if err := json.Unmarshal([]byte(wrap.B), &res); err != nil {
		return 0, err
	}
	return res.FailedCount, nil
}

// WaitLogin, oturumun kurulmasını bekler (CAPTCHA çözümü sonrası).
func WaitLogin(b *kahin.Browser, timeout time.Duration) bool {
	return waitFor(b, timeout, func() bool { return loginSucceeded(b) }) == nil
}

// loginSucceeded, oturumun kurulduğunu söyler.
func loginSucceeded(b *kahin.Browser) bool {
	out, err := b.EvalString(`(function(){
		var u=location.href;
		if(u.indexOf('gitee.com/login')>=0||u.indexOf('/oauth/authorize')>=0||u.indexOf('tox_token')>=0)return 'no';
		var t=document.title||'';
		if(t.indexOf('工作台')>=0||t.indexOf('Dashboard')>=0||t.indexOf('Gitee.com')>=0)return 'yes';
		if(document.querySelector('a[href*="/logout"],[href*="/notifications"]'))return 'yes';
		return 'no';
	})()`)
	if err != nil {
		return false
	}
	return strings.Trim(out, `"`) == "yes"
}

// loginRejected, girişin hata metniyle reddedildiğini söyler.
func loginRejected(b *kahin.Browser) bool {
	out, err := b.EvalString(`(function(){
		var t=(document.body?document.body.innerText:'');
		if(/Invalid email or password|Invalid password|密码错误|用户名或密码错误|验证码不正确/.test(t))return 'yes';
		return 'no';
	})()`)
	if err != nil {
		return false
	}
	return strings.Trim(out, `"`) == "yes"
}

// waitFor, koşul sağlanana kadar bekler.
func waitFor(b *kahin.Browser, timeout time.Duration, cond func() bool) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("koşul zaman aşımı")
}

// evalBool, sayfada boolean JS ifadesi çalıştırır.
func evalBool(b *kahin.Browser, expr string) bool {
	out, err := b.EvalString("(function(){return (" + expr + ")?'yes':'no'})()")
	if err != nil {
		return false
	}
	return strings.Trim(out, `"`) == "yes"
}

// captchaWall, Gitee'nin Baidu slider/2FA duvarının görünür olup olmadığını söyler.
func captchaWall(b *kahin.Browser) (bool, string) {
	out, err := b.EvalString(`(function(){
		function vis(sel){var e=document.querySelector(sel);if(!e)return false;var r=e.getBoundingClientRect();return r.width>0&&r.height>0}
		var txt=(document.body?document.body.innerText:'').slice(0,4000);
		if(/Machine verification|Security Verification|Drag the slider|验证失败|安全验证/.test(txt))return 'slider';
		if(vis('.session__2verify')||vis('.baidu-captcha-container'))return 'slider';
		return '';
	})()`)
	if err != nil {
		return false, ""
	}
	kind := strings.Trim(out, `"`)
	return kind != "", kind
}

// fillJS, bir input'a değeri native setter + input/change olaylarıyla yazar.
//
// Bu, Gitee gibi sayfa JS'ine bağlı şifreleme alanları için ZORUNLUDUR:
// dom_action "type" olay tetiklemediği için encrypt_data boş kalır.
func fillJS(sel, val string) string {
	return fmt.Sprintf(`(function(){
		var e=document.querySelector(%q);
		if(!e)return 'no-el';
		var d=Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype,'value');
		if(d&&d.set){d.set.call(e,%q)}else{e.value=%q}
		e.dispatchEvent(new Event('input',{bubbles:true}));
		e.dispatchEvent(new Event('change',{bubbles:true}));
		return 'ok';
	})()`, sel, val, val)
}

// giteeAuthorize, OAuth onay ekranını geçer.
//
// Girişten sonra otomatik onay sayfası gelir; "Permmit" gönderilir.
// Zaten onaylanmışsa (callback'e dönülmüşse) bu adım atlanır.
func (s *GitCode) GiteeAuthorize(ctx context.Context, b *kahin.Browser) error {
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		info, err := b.Info()
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		// Onay ekranı.
		if strings.Contains(info.URL, "gitee.com/oauth/authorize") ||
			strings.Contains(info.Body, "OAuth 授权请求") {
			if err := b.ClickSelector(`input[value=Permmit]`, 15*time.Second); err == nil {
				time.Sleep(3 * time.Second)
				continue
			}
			// İngilizce arayüzde buton "Authorize" olabilir.
			if err := b.ClickSelector(`input[value=Authorize]`, 10*time.Second); err == nil {
				time.Sleep(3 * time.Second)
				continue
			}
		}
		// Callback'e döndüyse bitti.
		if strings.Contains(info.URL, "gitcode.com/oauth/callback") ||
			strings.Contains(info.URL, "gitcode.com") {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("gitee OAuth onayı tamamlanmadı (son: %s)", rURL(b))
}

// gitcodeRegister, callback sonrası kayıt formunu doldurur.
//
// Form: Set username (ön dolu), ülke kodu seçici (varsayılan +86),
// Verify phone number, Verification code + "Get verification code",
// iki onay kutusu, "Create account and continue".
//
// CANLI DOĞRULANAN MEKANİKLER:
//   - Ülke kodu seçici bir devui-select'tir; varsayılan "+86"dır ve seçici
//     yalnızca gerçek pointer olayıyla (dom_action click) açılır. Sentetik
//     MouseEvent/click() seçiciyi DEĞİŞTİRMEZ. Numara yanlış ülkeye gitmesin
//     diye seçici mutlaka phone.CC'ye ayarlanır.
//   - "Get verification code" düğmesi (.g-input-button-append) telefon
//     alanı geçerli dolana kadar pasiftir (cursor: not-allowed); dolunca
//     "-active" sınıfı gelir. Bu yüzden önce aktifleşmesi beklenir.
//   - Onay kutuları görünmez input'lardır (0x0, opacity 0); programatik
//     .click() devui'nin change olayını tetikler ve checked=true olur.
func (s *GitCode) gitcodeRegister(ctx context.Context, b *kahin.Browser, acc accounts.Account, phone sms.Number, codeFn func(context.Context, sms.Number) (string, error)) error {
	// Formun gelmesini bekle.
	var snap *kahin.Snapshot
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		snap, err = b.Snapshot("", 4000)
		if err == nil {
			nodes := kahin.ParseSnapshot(snap.Lines)
			if hasPhoneField(nodes) {
				break
			}
		}
		time.Sleep(2 * time.Second)
	}
	if snap == nil {
		return fmt.Errorf("gitcode kayıt formu gelmedi")
	}
	nodes := kahin.ParseSnapshot(snap.Lines)

	phoneBox := findTextbox(nodes, "Verify phone number")
	codeBox := findTextbox(nodes, "Verification code")
	if phoneBox == nil || codeBox == nil {
		return fmt.Errorf("gitcode form alanları bulunamadı (phone=%v code=%v)", phoneBox != nil, codeBox != nil)
	}

	// Ülke kodu: seçici +86'da kalırsa numara yanlış ülkeye gider. Doğru
	// ülkeye ayarla (numara zaten o ülkeye ait).
	if err := selectCountryCode(b, phone.CC); err != nil {
		return fmt.Errorf("ülke kodu %s seçilemedi: %w", phone.CC, err)
	}

	local := phone.Local
	if local == "" {
		local = strings.TrimPrefix(phone.E164, "+")
	}
	if err := b.FillForm([]kahin.Field{{Ref: phoneBox.Ref, Text: local}}, 15*time.Second); err != nil {
		return fmt.Errorf("telefon yazılamadı: %w", err)
	}

	// "Get verification code": telefon dolunca aktifleşir; aktifleşmesini bekle.
	if err := clickGetCode(b); err != nil {
		return fmt.Errorf("kod isteme düğmesi: %w", err)
	}

	// SMS gönderiminden önce NetEase Yidun CAPTCHA'sı çıkabilir (canlı
	// doğrulandı: "Get verification code" tıklaması c.dun.163.com istekleri
	// üretir). CAPTCHA görünürse Grok CLI ile çözülür; ancak ondan sonra SMS
	// gönderilir.
	if waitFor(b, 6*time.Second, func() bool { return yidunVisible(b) }) == nil {
		if s.Solver == nil {
			return fmt.Errorf("SMS kodu için NetEase Yidun CAPTCHA çıktı, çözücü yok")
		}
		if err := SolveYidun(ctx, b, s.Solver, rURL(b)); err != nil {
			return fmt.Errorf("Yidun CAPTCHA çözülemedi: %w", err)
		}
		// CAPTCHA çözümü sonrası kod isteği tamamlanmış olmalı; değilse
		// düğmeye yeniden bas.
		if !getCodeActive(b) {
			_ = clickGetCode(b)
		}
	}

	// SMS kodunu bekle.
	code, err := codeFn(ctx, phone)
	if err != nil {
		return fmt.Errorf("SMS kodu alınamadı: %w", err)
	}

	// Kod alanına yaz (yeni snapshot: DOM değişmiş olabilir).
	snap2, err := b.Snapshot("", 4000)
	if err != nil {
		return err
	}
	nodes2 := kahin.ParseSnapshot(snap2.Lines)
	codeBox2 := findTextbox(nodes2, "Verification code")
	if codeBox2 == nil {
		codeBox2 = codeBox
	}
	if err := b.FillForm([]kahin.Field{{Ref: codeBox2.Ref, Text: code}}, 15*time.Second); err != nil {
		return fmt.Errorf("kod yazılamadı: %w", err)
	}

	// Onay kutularını işaretle (iki adet) ve gerçekten işaretlendiğini doğrula.
	if err := checkAllCheckboxes(b); err != nil {
		return fmt.Errorf("onay kutuları: %w", err)
	}

	// Gönder.
	if err := clickText(b, "Create account and continue"); err != nil {
		return fmt.Errorf("kayıt düğmesi: %w", err)
	}

	// Sonucu doğrula: form gitti mi, yoksa hata mesajı mı var?
	deadline = time.Now().Add(45 * time.Second)
	var lastBody string
	for time.Now().Before(deadline) {
		info, err := b.Info()
		if err == nil {
			lastBody = info.Body
			if msg := registerError(info.Body); msg != "" {
				return fmt.Errorf("gitcode kayıt reddedildi: %s", msg)
			}
			if !strings.Contains(info.URL, "/oauth/callback") &&
				!strings.Contains(info.Body, "Create account and continue") {
				return nil // form gitti, kayıt tamam
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("kayıt sonucu belirsiz (son: %s | %s)", rURL(b), truncateOne(lastBody, 160))
}

// registerError, kayıt formundaki bilinen hata metinlerini arar.
func registerError(body string) string {
	marks := []string{
		"Please read and agree",
		"Invalid verification code",
		"verification code is invalid",
		"Verification code error",
		"手机号格式",
		"验证码错误",
		"该手机号已被注册",
		"already been registered",
		"already registered",
		"验证码已过期",
		"验证码失效",
	}
	for _, m := range marks {
		if strings.Contains(body, m) {
			return m
		}
	}
	return ""
}

// selectCountryCode, gitcode kayıt formundaki ülke kodu seçicisini cc'ye ayarlar.
//
// CANLI DOĞRULANAN MEKANİK (bu oturum, kanıtlandı):
//   - Seçici: div.devui-select > input.devui-select__input (readonly, "+86").
//     Form gelir gelmez seçici HENÜZ DOM'da olmayabilir; beklenmeli.
//   - Açma: gerçek tıklama (mirage_click native koordinat yolu) listeyi açar.
//   - Seçme: li öğesi gerçek fare koordinatıyla SEÇİLMEZ (devui kendi olay
//     sırasını bekler). Snapshot ref'i + kahin_mirage_dom_action("click")
//     SEÇER — canlı kanıt: +86 → +44 değişti.
//   - Doğrulama: input değeri "+<cc>" olur.
func selectCountryCode(b *kahin.Browser, cc string) error {
	if cc == "" {
		return fmt.Errorf("ülke kodu boş")
	}
	want := "+" + cc
	if cur, _ := countryValue(b); cur == want {
		return nil
	}
	// Seçicinin DOM'a gelmesini bekle (form ile aynı anda gelmez).
	if waitFor(b, 15*time.Second, func() bool {
		return evalBool(b, `!!document.querySelector('input.devui-select__input')`)
	}) != nil {
		return fmt.Errorf("ülke kodu seçicisi görünmedi")
	}
	// 1) Seçiciyi aç (gerçek tıklama).
	if err := b.ClickSelector("input.devui-select__input", 10*time.Second); err != nil {
		return fmt.Errorf("seçici açılamadı: %w", err)
	}
	// 2) Liste öğesini snapshot ref'i ile bul ve dom_action ile seç.
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		snap, err := b.Snapshot("", 9000)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		opt := findCountryOption(kahin.ParseSnapshot(snap.Lines), cc)
		if opt == nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if _, err := b.DOMAction(opt.Ref, "click", ""); err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if waitFor(b, 6*time.Second, func() bool {
			cur, _ := countryValue(b)
			return cur == want
		}) == nil {
			return nil
		}
	}
	cur, _ := countryValue(b)
	return fmt.Errorf("ülke kodu +%s ayarlanamadı (mevcut: %s)", cc, cur)
}

// findCountryOption, etiketi "+<cc>" ile biten liste öğesini döndürür.
//
// Snapshot li etiketi "United Kingdom +44" biçimindedir (satır içi boşluk
// tek boşluğa indirgenir).
func findCountryOption(nodes []kahin.SnapNode, cc string) *kahin.SnapNode {
	suffix := "+" + cc
	for i := range nodes {
		if !strings.EqualFold(nodes[i].Role, "li") {
			continue
		}
		lbl := strings.TrimSpace(nodes[i].Label)
		if lbl != "" && strings.HasSuffix(lbl, suffix) {
			return &nodes[i]
		}
	}
	return nil
}

// elementCenter, CSS selector'a uyan öğenin görüntü alanı merkezini döndürür.
func elementCenter(b *kahin.Browser, sel string) ([]float64, error) {
	out, err := b.EvalString(fmt.Sprintf(`(function(){
		var e=document.querySelector(%q);
		if(!e)return 'null';
		var r=e.getBoundingClientRect();
		if(r.width<=0||r.height<=0)return 'null';
		return JSON.stringify([r.left+r.width/2, r.top+r.height/2]);
	})()`, sel))
	if err != nil {
		return nil, err
	}
	s := strings.Trim(out, `"`)
	if s == "null" || s == "" {
		return nil, fmt.Errorf("öğe görünür değil: %s", sel)
	}
	var v []float64
	if err := json.Unmarshal([]byte(s), &v); err != nil || len(v) != 2 {
		return nil, fmt.Errorf("koordinat çözülemedi: %s", s)
	}
	return v, nil
}

// countryValue, ülke kodu girişinin geçerli değerini okur.
func countryValue(b *kahin.Browser) (string, error) {
	return b.EvalString(`(function(){var e=document.querySelector('input.devui-select__input');return e?e.value:''})()`)
}

// getCodeActive, "Get verification code" düğmesinin aktif olup olmadığını söyler.
//
// CANLI DOĞRULANAN MEKANİK: düğme pasifken sınıfı
// "g-input-button-append g-input-button-append-inactive", aktifken
// "g-input-button-append g-input-button-append-active" olur.
//
// DİKKAT: "append-inactive" içinde "-active" alt dizesi GEÇER; bu yüzden
// yalnızca "append-active" aranmalıdır (eski kod "-active" arıyordu ve pasif
// düğmeyi de aktif sanıyordu — düzeltildi).
func getCodeActive(b *kahin.Browser) bool {
	return evalBool(b, `(function(){
		var e=document.querySelector('.g-input-button-append');
		if(!e)return false;
		return String(e.className).indexOf('append-active')>=0;
	})()`)
}

// clickGetCode, "Get verification code" düğmesine basar.
//
// Düğme telefon geçerli dolana kadar pasiftir (not-allowed); aktifleşmesini
// bekler, sonra gerçek pointer ile tıklar.
func clickGetCode(b *kahin.Browser) error {
	// Aktifleşmesini bekle.
	active := false
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if getCodeActive(b) {
			active = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !active {
		return fmt.Errorf("kod isteme düğmesi aktifleşmedi (telefon alanı geçersiz olabilir)")
	}
	// Gerçek pointer ile tıkla.
	if err := b.ClickSelector(".g-input-button-append", 10*time.Second); err == nil {
		return nil
	}
	// Yedek: snapshot'taki metinden bul.
	if err := clickText(b, "Get verification code"); err == nil {
		return nil
	}
	return fmt.Errorf("kod isteme düğmesine basılamadı")
}

func truncateOne(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ---------------------------------------------------------------------------
// yardımcılar
// ---------------------------------------------------------------------------

func findTextbox(nodes []kahin.SnapNode, placeholder string) *kahin.SnapNode {
	if n := kahin.FindRole(nodes, "textbox", placeholder); n != nil {
		return n
	}
	// Yerelleştirilmiş etiketler.
	alt := map[string]string{
		"Verify phone number": "手机号",
		"Verification code":   "验证码",
	}
	if a, ok := alt[placeholder]; ok {
		return kahin.FindRole(nodes, "textbox", a)
	}
	return nil
}

func hasPhoneField(nodes []kahin.SnapNode) bool {
	return findTextbox(nodes, "Verify phone number") != nil
}

// clickText, snapshot'ta verilen metni içeren tıklanabilir düğümü bulup tıklar.
func clickText(b *kahin.Browser, text string) error {
	// Önce locator ile dene (text= zinciri).
	if err := b.ClickSelector("text="+text, 12*time.Second); err == nil {
		return nil
	}
	snap, err := b.Snapshot("", 4000)
	if err != nil {
		return err
	}
	nodes := kahin.ParseSnapshot(snap.Lines)
	if n := kahin.FindClickable(nodes, text); n != nil {
		_, err := b.DOMAction(n.Ref, "click", "")
		return err
	}
	return fmt.Errorf("tıklanabilir düğüm bulunamadı: %q", text)
}

// checkAllCheckboxes, sayfadaki tüm onay kutularını işaretler ve doğrular.
//
// Onay kutuları görünmez input'lardır (0x0, opacity 0). Programatik .click()
// devui'nin change olayını tetikler ve checked=true olur; bu canlı doğrulandı.
// İşaretleme sonrası her kutunun checked olduğu kontrol edilir.
func checkAllCheckboxes(b *kahin.Browser) error {
	out, err := b.EvalString(`(function(){
		var boxes=Array.from(document.querySelectorAll('input[type=checkbox]'));
		var n=0;
		boxes.forEach(function(c){ if(!c.checked){ c.click(); n++; } });
		return JSON.stringify({count:boxes.length,clicked:n,checked:boxes.map(function(c){return c.checked})});
	})()`)
	if err != nil {
		return err
	}
	if strings.Contains(out, `"count":0`) {
		return fmt.Errorf("onay kutusu yok")
	}
	if !strings.Contains(out, "true") {
		return fmt.Errorf("onay kutuları işaretlenemedi: %s", out)
	}
	if strings.Contains(out, "false") {
		return fmt.Errorf("bazı onay kutuları işaretsiz kaldı: %s", out)
	}
	return nil
}

func rURL(b *kahin.Browser) string {
	info, err := b.Info()
	if err != nil {
		return "?"
	}
	return info.URL
}

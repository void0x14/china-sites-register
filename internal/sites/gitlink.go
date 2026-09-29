package sites

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/void0x14/china-sites-register/internal/accounts"
	"github.com/void0x14/china-sites-register/internal/captcha"
	"github.com/void0x14/china-sites-register/internal/kahin"
	"github.com/void0x14/china-sites-register/internal/sms"
)

// ---------------------------------------------------------------------------
// GitLink (gitlink.org.cn)
//
// CANLI DOĞRULANAN (bu oturum):
//   - https://gitlink.org.cn/uc/api/v1/oauth/login/gitee → /nopage
//     ("你访问的页面不存在" — sayfa yok). Bu uç GitLink'te YOKTUR.
//   - https://gitlink.org.cn/login → "快速登录" panelinde YALNIZCA
//     ccf / educoder / qq / github sağlayıcıları vardır. GITEE YOK.
//   - Bu yüzden GitLink'te "Gitee ile kayıt ol" akışı MÜMKÜN DEĞİLDİR.
//
// GitLink adaptörü, Gitee hesabı yerine klasik kayıt formunu (/register)
// kullanır: kullanıcı adı + e-posta + şifre (hesaplar.txt 4. alanı).
// ---------------------------------------------------------------------------

// GitLink, gitlink.org.cn adaptörüdür.
type GitLink struct {
	Solver *captcha.Solver
}

func (s *GitLink) Name() string { return "gitlink" }

// OAuthLoginURL, GitLink'te Gitee OAuth bulunmadığı için kayıt sayfasını
// döndürür (arayüz gereği; Gitee OAuth akışı yoktur).
func (s *GitLink) OAuthLoginURL() string {
	return "https://gitlink.org.cn/register"
}

// SupportedCCs, GitLink kayıt formunun ülke kodlarıdır.
func (s *GitLink) SupportedCCs() []string {
	return []string{"86"}
}

// Register, GitLink klasik kayıt akışını sürer.
//
// GitLink'te Gitee OAuth yoktur (canlı doğrulandı); bu yüzden Gitee
// hesabının kullanıcı adı + e-postası + şifresi (4. alan) ile klasik
// kayıt formu doldurulur.
func (s *GitLink) Register(ctx context.Context, b *kahin.Browser, acc accounts.Account, phone sms.Number, codeFn func(context.Context, sms.Number) (string, error)) (*Result, error) {
	r := &Result{Site: s.Name(), Account: acc, Phone: phone}

	if err := b.Navigate("https://gitlink.org.cn/register", "domcontentloaded", 45*time.Second); err != nil {
		r.Stage = "kayıt_sayfası"
		r.Err = fmt.Errorf("gitlink kayıt sayfası: %w", err)
		return r, r.Err
	}
	if waitFor(b, 25*time.Second, func() bool {
		return evalBool(b, `document.querySelectorAll('input').length>0`)
	}) != nil {
		r.Stage = "kayıt_sayfası"
		r.Err = fmt.Errorf("gitlink kayıt formu gelmedi (url: %s)", rURL(b))
		return r, r.Err
	}

	// Form alanlarını canlı DOM'dan bul (id'ler sürümle değişebilir).
	if err := fillGenericRegister(b, acc); err != nil {
		r.Stage = "kayıt_formu"
		r.Err = err
		return r, r.Err
	}
	info, _ := b.Info()
	r.Evidence = info.URL + " | " + info.Title

	deadline := time.Now().Add(40 * time.Second)
	var lastBody string
	for time.Now().Before(deadline) {
		if info, err := b.Info(); err == nil {
			lastBody = info.Body
			if msg := registerError(info.Body); msg != "" {
				r.Stage = "kayıt_formu"
				r.Err = fmt.Errorf("gitlink kayıt reddedildi: %s", msg)
				return r, r.Err
			}
			if !strings.Contains(info.URL, "/register") {
				r.Success = true
				r.Stage = "tamamlandı"
				r.Evidence = info.URL + " | " + info.Title
				return r, nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	r.Stage = "kayıt_formu"
	r.Err = fmt.Errorf("gitlink kayıt sonucu belirsiz (son: %s | %s)", rURL(b), truncateOne(lastBody, 160))
	return r, r.Err
}

// fillGenericRegister, alan adlarından (name/id/placeholder) yararlanarak
// genel bir kayıt formunu doldurur: kullanıcı adı, e-posta, şifre.
//
// Alanlar TAHMİN EDİLMEZ: canlı DOM'dan rolüne göre bulunur (type=email →
// e-posta; type=password → şifre; type=text ve adında user/name/login →
// kullanıcı adı). Bulunamayan alan hata verir (sessiz başarı yok).
func fillGenericRegister(b *kahin.Browser, acc accounts.Account) error {
	login := acc.Username
	if login == "" {
		login = strings.Split(acc.Email, "@")[0]
	}
	script := fmt.Sprintf(`(function(){
		function set(e,val){
			var d=Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype,'value');
			if(d&&d.set){d.set.call(e,val)}else{e.value=val}
			e.dispatchEvent(new Event('input',{bubbles:true}));
			e.dispatchEvent(new Event('change',{bubbles:true}));
		}
		function visible(e){var r=e.getBoundingClientRect();return r.width>0&&r.height>0}
		var ins=Array.from(document.querySelectorAll('input')).filter(visible);
		var email=null,pw=null,user=null,confirm=null;
		ins.forEach(function(e){
			var sig=(e.type+' '+(e.name||'')+' '+(e.id||'')+' '+(e.placeholder||'')).toLowerCase();
			if(e.type==='email'||(sig.indexOf('email')>=0&&e.type!=='password')){if(!email)email=e}
			else if(e.type==='password'){
				if(sig.indexOf('confirm')>=0||sig.indexOf('again')>=0||sig.indexOf('repeat')>=0||sig.indexOf('确认')>=0){if(!confirm)confirm=e}
				else if(!pw)pw=e;
			}
			else if(e.type==='text'||e.type==='tel'){
				if(sig.indexOf('user')>=0||sig.indexOf('name')>=0||sig.indexOf('login')>=0||sig.indexOf('账号')>=0||sig.indexOf('用户')>=0){if(!user)user=e}
			}
		});
		var ok={email:false,password:false,username:false};
		if(user){set(user,%q);ok.username=true}
		if(email){set(email,%q);ok.email=true}
		if(pw){set(pw,%q);ok.password=true}
		if(confirm){set(confirm,%q)}
		// Şartlar onayı varsa işaretle.
		document.querySelectorAll('input[type=checkbox]').forEach(function(c){if(!c.checked)c.click()});
		var f=document.querySelector('form');
		if(f){
			var btn=f.querySelector('input[type=submit],button[type=submit]');
			if(btn){btn.click();ok.submitted=true}
			else if(f.requestSubmit){f.requestSubmit();ok.submitted=true}
		}
		return JSON.stringify(ok);
	})()`, login, acc.Email, acc.Password, acc.Password)
	out, err := b.EvalString(script)
	if err != nil {
		return fmt.Errorf("gitlink form doldurulamadı: %w", err)
	}
	for _, k := range []string{"username", "email", "password"} {
		if !strings.Contains(out, `"`+k+`":true`) {
			return fmt.Errorf("gitlink form alanı bulunamadı (%s): %s", k, truncateOne(out, 200))
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// JiHuLab (jihulab.com — GitLab CE, 极狐)
//
// CANLI DOĞRULANAN: https://jihulab.com/users/auth/gitee → HTTP 401
// (Gitee OAuth sağlayıcısı yapılandırılmamış). Kayıt sayfası
// https://jihulab.com/users/sign_up açıktır (200) ve e-posta + kullanıcı adı
// + şifre ister; telefon isteğe bağlıdır.
// ---------------------------------------------------------------------------

// JiHuLab, jihulab.com adaptörüdür.
type JiHuLab struct {
	Solver *captcha.Solver
}

func (s *JiHuLab) Name() string { return "jihulab" }

// OAuthLoginURL, JiHuLab'in Gitee OAuth girişidir.
func (s *JiHuLab) OAuthLoginURL() string {
	return "https://jihulab.com/users/auth/gitee"
}

// SupportedCCs, JiHuLab kayıt formunun ülke kodlarıdır.
//
// JiHuLab bir GitLab kurulumudur; telefon alanı ülke kodu seçicisi
// GitLab'ın kendi listesidir. Numara gerekmeyebilir; SMS akışı için
// yaygın ülkeler desteklenir.
func (s *JiHuLab) SupportedCCs() []string {
	return []string{"86"}
}

// Register, JiHuLab kayıt akışını sürer.
//
// CANLI DOĞRULANAN DOM (jihulab.com/users/sign_up):
//   - İki mod: "Register with phone" ve "Register with email".
//   - Telefon modu: #new_user_username (new_user[username]),
//     #new_user_password (new_user[password]),
//     #new_user_phone (new_user[phone]),
//     #new_user_verification_code (new_user[verification_code]),
//     #new-user-terms-accepted-1 (new_user_terms_accepted), submit=commit.
//   - Form action=/users, id=new_new_user.
//   - Gitee OAuth sağlayıcısı YOK (users/auth/gitee → 401); bu yüzden
//     Gitee hesabıyla doğrudan kayıt mümkün değildir. Telefon modu SMS
//     doğrulaması ister; Gitee e-postası + Gitee şifresi (4. alan) ile
//     e-posta modu kullanılır.
func (s *JiHuLab) Register(ctx context.Context, b *kahin.Browser, acc accounts.Account, phone sms.Number, codeFn func(context.Context, sms.Number) (string, error)) (*Result, error) {
	r := &Result{Site: s.Name(), Account: acc, Phone: phone}

	if err := b.Navigate("https://jihulab.com/users/sign_up", "domcontentloaded", 45*time.Second); err != nil {
		r.Stage = "kayıt_sayfası"
		r.Err = fmt.Errorf("jihulab kayıt sayfası: %w", err)
		return r, r.Err
	}
	if waitFor(b, 25*time.Second, func() bool {
		return evalBool(b, `!!document.querySelector('#new_user_username')`)
	}) != nil {
		r.Stage = "kayıt_sayfası"
		r.Err = fmt.Errorf("jihulab kayıt formu gelmedi (url: %s)", rURL(b))
		return r, r.Err
	}

	// E-posta moduna geç (Gitee hesabı e-posta + şifre ile kayıt).
	_, _ = b.EvalString(`(function(){
		var t=document.querySelector('#register-with-email, [data-testid="register-with-email"]');
		if(t){t.click();return 'ok'}
		return 'yok';
	})()`)
	time.Sleep(1 * time.Second)

	if err := fillJiHuLabForm(b, acc); err != nil {
		r.Stage = "kayıt_formu"
		r.Err = err
		return r, r.Err
	}
	info, _ := b.Info()
	r.Evidence = info.URL + " | " + info.Title

	// Sonucu doğrula.
	deadline := time.Now().Add(40 * time.Second)
	var lastBody string
	for time.Now().Before(deadline) {
		if info, err := b.Info(); err == nil {
			lastBody = info.Body
			if msg := registerError(info.Body); msg != "" {
				r.Stage = "kayıt_formu"
				r.Err = fmt.Errorf("jihulab kayıt reddedildi: %s", msg)
				return r, r.Err
			}
			if !strings.Contains(info.URL, "/users/sign_up") &&
				!strings.Contains(info.Body, "Register with") {
				r.Success = true
				r.Stage = "tamamlandı"
				r.Evidence = info.URL + " | " + info.Title
				return r, nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	r.Stage = "kayıt_formu"
	r.Err = fmt.Errorf("jihulab kayıt sonucu belirsiz (son: %s | %s)", rURL(b), truncateOne(lastBody, 160))
	return r, r.Err
}

// fillJiHuLabForm, GitLab kayıt formunu doldurur ve gönderir.
//
// CANLI DOĞRULANAN ID'ler: #new_user_username, #new_user_email,
// #new_user_password, #new-user-terms-accepted-1; form id=new_new_user.
func fillJiHuLabForm(b *kahin.Browser, acc accounts.Account) error {
	login := acc.Username
	if login == "" {
		login = strings.Split(acc.Email, "@")[0]
	}
	// Şifre: hesaplar.txt 4. alanı. GitLab en az 8 karakter ve sayı ister;
	// Gitee şifresi 10 karakter + rakam içeriyor (canlı doğrulandı).
	pass := acc.Password
	script := fmt.Sprintf(`(function(){
		function set(sel,val){
			var e=document.querySelector(sel);
			if(!e)return false;
			var d=Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype,'value');
			if(d&&d.set){d.set.call(e,val)}else{e.value=val}
			e.dispatchEvent(new Event('input',{bubbles:true}));
			e.dispatchEvent(new Event('change',{bubbles:true}));
			return true;
		}
		var ok={};
		ok.username=set('#new_user_username',%q);
		ok.email=set('#new_user_email',%q);
		ok.password=set('#new_user_password',%q);
		// Şartlar onayı.
		var cb=document.querySelector('#new-user-terms-accepted-1');
		if(cb&&!cb.checked){cb.click();}
		ok.terms=cb?cb.checked:false;
		var f=document.querySelector('form#new_new_user');
		if(f){
			var btn=f.querySelector('input[type=submit],button[type=submit]');
			if(btn){btn.click();ok.submitted=true}
			else if(f.requestSubmit){f.requestSubmit();ok.submitted=true}
		}
		return JSON.stringify(ok);
	})()`, login, acc.Email, pass)
	out, err := b.EvalString(script)
	if err != nil {
		return fmt.Errorf("jihulab form doldurulamadı: %w", err)
	}
	for _, k := range []string{"username", "email", "password"} {
		if !strings.Contains(out, `"`+k+`":true`) {
			return fmt.Errorf("jihulab form alanı yazılamadı (%s): %s", k, truncateOne(out, 160))
		}
	}
	return nil
}

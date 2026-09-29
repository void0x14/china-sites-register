// Package sites, hedef Çin git platformlarındaki "Gitee ile kayıt ol" (OAuth)
// akışını sürer.
//
// Desteklenen siteler: gitcode (AtomGit), gitlink (GitLink), jihulab (极狐).
package sites

import (
	"context"
	"time"

	"github.com/void0x14/china-sites-register/internal/accounts"
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

// All, desteklenen tüm site adaptörlerini döndürür.
//
// Her çağrıda taze adaptör üretir (Solver alanı işçiye göre atanır).
func All() []Site {
	return []Site{
		&GitCode{},
		&GitLink{},
		&JiHuLab{},
	}
}

// ByName, ada göre adaptör döndürür.
func ByName(name string) (Site, bool) {
	for _, s := range All() {
		if s.Name() == name {
			return s, true
		}
	}
	return nil, false
}

// Names, desteklenen site adlarını döndürür.
func Names() []string {
	var out []string
	for _, s := range All() {
		out = append(out, s.Name())
	}
	return out
}

// DefaultTimeout, tek bir site işlemi için önerilen üst sınırdır.
const DefaultTimeout = 20 * time.Minute

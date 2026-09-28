# Free Receive-SMS: Dünya Çapında Site / Sistem / API Kataloğu

**Derleme tarihi:** 2026-09-28
**Kaynak yöntemi:** Canlı web taraması (Exa search + fetch), birincil sayfalar ve API dokümanları.
**Kapsam:** Ücretsiz (kayıtsız, public inbox) SMS alma siteleri, freemium servisler, ücretli doğrulama API'leri, açık kaynak scraper/aggregator projeleri, self-hosted SMS gateway'ler.

---

## 0. Sınıflandırma

Bu alanda dört ayrı sistem tipi var, karıştırma:

| Tip | Ne yapar | Kayıt | Para | Inbox |
|---|---|---|---|---|
| **A. Public inbox siteleri** | Numarayı herkes görür, gelen SMS herkese açık sayfada | Yok | Yok | **Public** |
| **B. Freemium sağlayıcılar** | Ücretsiz numara havuzu + ücretli özel numara/API | Bazen | Kısmen | Public (free) / Private (paid) |
| **C. Ücretli doğrulama API'leri** | Numara satın al, OTP'yi private oku, REST API | Var | Var | **Private** |
| **D. Self-hosted gateway** | Kendi Android telefonunu/GSM modemini SMS sunucusu yap | — | 0 (kendi SIM'in) | Private |

**Kritik gerçek (2026):** A tipi sitelerin çoğu ölü veya major platformlarca (WhatsApp, Telegram, Google, Instagram, TikTok, X) bloklu. Numaralar yayınlandıktan saatler içinde kara listeye giriyor; çoğu VoIP olduğu için doğrulama adımında reddediliyor. Public inbox olduğu için kodu başkası kapabilir.

---

## 1. TİP A — Public Inbox Siteleri (ücretsiz, kayıtsız)

Canlı olarak erişilip içeriği doğrulananlar işaretli.

| # | Site | Ülke kapsamı | Notlar |
|---|---|---|---|
| 1 | **sms24.me** | 53 ülke, 10.498 public inbox | ✅ canlı doğrulandı. En geniş ülke listesi. `Last updated: July 2026`. Telegram: @sms24me |
| 2 | **receive-sms-free.cc** | 30+ ülke | ✅ canlı. Numara başına "1 hour ago / 4 month ago" canlılık etiketi. Otomatik refresh iddiası |
| 3 | **quackr.io/temporary-numbers** | 24 ülke, 1000+ numara | ✅ canlı. 2M+ kullanıcı iddiası. Mesajlar 24 saatte silinir, numaralar ≥30 günde rotasyon. Ortalama iddia: 8 sn teslim |
| 4 | **sms-online.co/receive-free-sms** | USA, DE, ES, RO, UK, FR, RU, IT, CN, JP | ✅ canlı. 6 numara görünür (US×2, PR, MY, UK, SE). Kayıt yok |
| 5 | **anonymsms.com** | AU, SE, DE, UK, US, CA | ✅ canlı. Free public + ücretli kiralama ($4.99–25). "Real SIM" iddiası |
| 6 | **receivesms.co** | US(126), CA(70), PR(21) + 25 ülke×1 | ✅ canlı. `/active-numbers/` alt sayfası var |
| 7 | **receive-sms-online.info** | SE, FI, NL, US, ES, RO, UK, DE, FR, RU | ✅ canlı. "Real SIM + modem" iddiası, dinamik sender ID. Yüzlerce numara listeli |
| 8 | **receivesmsonline.net** | US, UK, SE, CA, DE | ✅ canlı. Numara başına toplam mesaj sayacı |
| 9 | **freephonenum.com** | US(10), CA(5) + 600 arşiv | ✅ canlı. US/CA odaklı, voicemail de var |
| 10 | **receive-smss.com** | 50+ ülke | ✅ canlı. US, CA, DE, UK, DK, ES, CZ, UA, AT, IN, HR, IT, PT, FR numaraları listeli |
| 11 | **freeonlinephone.org** | CA, UK, US | ✅ canlı. Numara başına mesaj sayacı (örn. 15933) |
| 12 | **sms.sellaite.com** | (dinamik) | ✅ canlı. SMS + **sesli arama doğrulama** + SMS→email. Listede o an numara yoksa boş |
| 13 | **hs3x.com** | US, UK, AT, SE, BE | ✅ canlı. Aylık güncelleme iddiası |
| 14 | **7sim.net** | 103 ülke listesi, 49 numara aktif | ✅ canlı. Online/Archive ayrımı, "added X days ago" + "last seen" zaman damgaları |
| 15 | **receive-a-sms.com** | US, BR, AU, CA, UK | ✅ canlı. "Email ile alma" opsiyonu |
| 16 | **sms-receive.net** | UK, NL, FI | ✅ canlı. "Real SIM" + public. Mesajlar silinemez |
| 17 | **pingme.tel/receive-sms-online** | US, CA, UK, AU, SE, AT, NL | ✅ canlı. Free + ücretli özel numara ($0.5/ay'dan) |
| 18 | **online-sms.org** | US, CA (15 numara) | ✅ canlı |
| 19 | **smstome.com** | UK, FI, BE, NL, SI, PL | ✅ canlı. Mesajlar 2–3 gün, numara ~30 gün rotasyon. Temp email de var |
| 20 | **smstools.online** | US, CA, AU, DE, FR, BE | ✅ canlı (menü sayfası) |
| 21 | **receive-sms.cc** | US, UK, CA, CN, IN, FR | Literatürde geçiyor, doğrudan çekilmedi |
| 22 | **receive-sms-online.com** | çok ülke | Literatürde geçiyor |
| 23 | **smsreceivefree.com** | US, CA + permanent | Literatürde; 2026 testinde ölü/başarısız raporlandı |
| 24 | **getfreesmsnumber.com** | ~20 ülke | SMS + **voicemail**. Ülke başına ayrı URL (`/free-receive-sms-from-us`) |
| 25 | **mytempsms.com** | çok ülke | SMS + sesli mesaj |
| 26 | **receivesmsonline.in** | IN | Literatürde |
| 27 | **receivefreesms.com** / **.net** | çok ülke | Literatürde |
| 28 | **freereceivesms.com** | çok ülke | Literatürde |
| 29 | **receivesms.xyz** | — | Literatürde |
| 30 | **receive-smss.live** | — | Literatürde |
| 31 | **freesms-online.com/phone-numbers** | — | Literatürde |
| 32 | **receivesmss.net/numbers** | — | Literatürde |
| 33 | **smsnumbersonline.com** | — | Literatürde |
| 34 | **smslisten.com** | — | ⚠️ çekildi, boş sayfa döndü |
| 35 | **smstibo.com** | — | ⚠️ çekildi, "click here to enter" ara sayfası |
| 36 | **catchsms.com** | — | ⚠️ çekildi, sadece cookie/reklam banner |
| 37 | **freesmscode.com** | — | ⚠️ çekildi, "getting things ready" (JS yükleyici) |
| 38 | **receive-sms-now.com** | RU dahil | Literatürde |
| 39 | **smska.us** | RU | Literatürde |
| 40 | **tempsms.ru** | RU | Literatürde |
| 41 | **getsms.org** | — | Literatürde |
| 42 | **onlinesim.ru** (free bölüm) | RU + global | Ücretli API'nin free kısmı |
| 43 | **smsc.ru** | RU/UA | Kayıt gerekli |
| 44 | **zadarma.com** | RU | Kayıt gerekli |
| 45 | **proovl.com/numbers** | — | Literatürde |
| 46 | **sellaite.com** (ana) | — | sms.sellaite.com ile aynı sağlayıcı |
| 47 | **receive-sms-free.com** | — | Literatürde |
| 48 | **receivesmsverification.com** | — | Literatürde |
| 49 | **freevirtualnumber.skycallbd.com** | BD | Literatürde |
| 50 | **anon-sms.com** | — | Literatürde |
| 51 | **mytrashmobile.com** / es.mytrashmobile.com | ES | Literatürde |
| 52 | **receive-sms.com** | CH dahil | Literatürde |
| 53 | **smsver.com** | — | Literatürde |
| 54 | **groovl.com** | — | Literatürde |
| 55 | **sendsmsnow.com** | — | Literatürde |
| 56 | **hidemynumbers.com** | — | Literatürde |
| 57 | **freeonlinephone.org** | — | (bkz. #11) |
| 58 | **1s2u.com** | — | Literatürde |
| 59 | **virtty.com** | — | Literatürde |
| 60 | **textanywhere.net** | — | Literatürde |
| 61 | **sms.ndtan.net** | — | Literatürde |
| 62 | **smstibo.com** | — | (bkz. #35) |
| 63 | **receivesmsnumber.com** | — | Literatürde |
| 64 | **smsreceiving.com** | — | Literatürde |
| 65 | **receiveasms.com** | — | Literatürde |
| 66 | **receiveasmsonline.com** | — | Literatürde |
| 67 | **receive-smss-online.com** | — | Literatürde |
| 68 | **en.yinsiduanxin.com** | CN | Literatürde |
| 69 | **sms-online.pl** | PL | Literatürde |
| 70 | **online-sms.org** | — | (bkz. #18) |
| 71 | **temp-sms.org** | NZ only | Literatürde |
| 72 | **ireceivesms.com** | US, UK, CA | Literatürde |
| 73 | **sms-receive.com** | — | Literatürde |
| 74 | **s-sms.com** | — | Literatürde |
| 75 | **smsreceiveonline.com** | — | Literatürde |
| 76 | **receivesmsonline.me** / **.eu** | — | Literatürde |
| 77 | **mytempsms.com** | — | (bkz. #25) |
| 78 | **smsget.net** | Megafon/Beeline | Literatürde |
| 79 | **mfreesms.com** | — | Literatürde |
| 80 | **numberforsms.com** | RU | Literatürde |
| 81 | **freevirtualsmsnumber.com** | — | Literatürde |
| 82 | **receive-smsonline.net** | — | Literatürde |
| 83 | **sms-online.co** | — | (bkz. #4) |
| 84 | **temp-number.com** | 20+ | Public free + **tam REST API** (bkz. §3) |
| 85 | **temp-number.org** | US, UK, CA, FR, DE, ES | Public free |
| 86 | **smstools.online** | — | (bkz. #20) |
| 87 | **esimplus.me** | — | eSIM + SMS |
| 88 | **getfreesmsnumber.com** | — | (bkz. #24) |
| 89 | **textnow.com** | US, CA | VoIP, ücretsiz, kayıt gerekli — major platformlarca bloklu |
| 90 | **textfree (pinger.com)** | US | VoIP, kayıt gerekli |
| 91 | **google voice** | US only | Gerçek özel US numarası; kurulum için mevcut US numarası şart |
| 92 | **receivesmsonline.info** | — | (bkz. #7) |

> **Not:** 21–92 arası girdiler çoğunlukla karşılaştırma listelerinden (blog/awesome-list) derlendi; canlılık doğrulaması yapılmadı. 1–20 canlı çekildi.

### 1.2 İkinci Tur Taramada Bulunan Ek Public Inbox Siteleri

| # | Site | Ülke kapsamı | Notlar |
|---|---|---|---|
| 93 | **zusms.com/en** | 20 ülke (US, UK, HK, KR, CA) | Public/shared inbox |
| 94 | **dogesms.com/ru/free-sms** | çok ülke | Public/shared inbox |
| 95 | **smsfox.net** | 86 ülke | Public/shared inbox |
| 96 | **text-verification.net** | 50+ ülke | Public/shared inbox |
| 97 | **proxied.com/ru/tools/receive-sms-online** | 16 ülke | Public/shared inbox |
| 98 | **receiveasmsonline.com** | 18+ ülke | Public/shared inbox |
| 99 | **smsfast.com/free-numbers** | 190+ ülke (10M+ numara iddiası) | Public/shared inbox |
| 100 | **tempsmss.com** | US, UK, CA, SE, FI, BE, NL, SI | Public/shared inbox |
| 101 | **receive-sms.io** | US, UK, AU + 40 ülke | Public/shared inbox |
| 102 | **receivesms.one** | 33+ ülke | Public/shared inbox |
| 103 | **sms-ol.com** | global (ID, UK, JP örnek) | Public/shared inbox |
| 104 | **smsonline.cloud** | 20+ ülke (CN, HK, JP, TW odak) | Public/shared inbox |
| 105 | **temporary-phone-number.io** | US, UK, CA, AU + 50+ | Public/shared inbox |
| 106 | **smsreceive.live** | 23+ ülke | Public/shared inbox |
| 107 | **smss.net** | 16+ ülke | Public/shared inbox |
| 108 | **instant-sms.com** | 100+ ülke | Public/shared inbox |
| 109 | **smspva.com/free-phone-numbers** | canlı liste | Public/shared inbox |
| 110 | **free-sms-receive.com** | canlı liste | Free numara; public inbox teyit edilmedi |
| 111 | **tiger-sms.com/free** | canlı liste | Free numara; public inbox teyit edilmedi |
| 112 | **numerotemporal.com** | canlı liste | Free numara; public inbox teyit edilmedi |
| 113 | **temporarynumber.com** | canlı liste | Free numara; public inbox teyit edilmedi |
| 114 | **receivesms.org** | canlı liste | Public/shared inbox |
| 115 | **receive-sms.app** | 22 ülke | Public/shared inbox; 2026 testinde ~18sn ile en hızlı |
| 116 | **virtualsms.io/free-numbers** | 19 ülke | Public/shared inbox |
| 117 | **mobilesms.io/free** | canlı liste | Public/shared inbox |
| 118 | **numbers.sms-bus.com** | 20 ülke | Free numara; public inbox teyit edilmedi |
| 119 | **tempsmsonline.com** | canlı liste | Free numara; public inbox teyit edilmedi |
| 120 | **spoofbox.com/en/tool/trash-mobile** | — | Public/shared inbox |
| 121 | **mytrashmobile.com** | — | Public/shared inbox |

> **Uyarı (agent doğrulaması):** `receive-sms.co`, `receive-sms-online.com`, `freereceivesms.com`, `freephonenumber.com` alan adları bağımsız olarak operasyonel public inbox olarak teyit EDİLEMEDİ. `receivesms.co`, `receive-sms-online.info` ve `freephonenum.com` bunlardan ayrı, farklı alan adlarıdır — karıştırma.

---

## 2. TİP B — Freemium (ücretsiz havuz + ücretli özel numara)

| Servis | Free | Ücretli taraf | API | Notlar |
|---|---|---|---|---|
| **quackr.io** | 1000+ paylaşımlı numara | Özel non-VoIP numara | — | En yüksek trafikli free sitelerden |
| **anonymsms.com** | Public numaralar | Kiralama: AU $10/gün, US $9/gün, CA $4.99/gün, UK $7.90/gün (30 gün $25) | — | "Pay per activation" da var |
| **pingme.tel** | Public numaralar | Özel numara $0.5/ay'dan, 20+ ülke | — | Telegram kanalı ile yeni numara duyurusu |
| **textnow.com** | US/CA VoIP | Premium | — | Major platformlarca bloklu |
| **hushed.com** | Trial | 60+ ülke, kiralama | — | Kayıt + ödeme |
| **burnerapp.com** | 7 gün trial | US/CA | — | Kısa projeler |
| **google voice** | US ücretsiz | — | — | Kurulum için mevcut US numarası şart |
| **onlinesim.io** | Free numara bölümü | Kiralama + aktivasyon | ✅ REST | Free numaralar public; API free listesi verir |
| **sms-man.com/free-numbers** | Free bölüm | 500+ servis, 350+ ülke | ✅ | Kayıt gerekmez (free için) |
| **sms-activate** | Free numara testi | 170+ ülke | ✅ | Bkz. §3 |
| **5sim.net/v1/guest/free** | Guest free endpoint | 150 ülke | ✅ | Bkz. §3 |
| **mobileSMS.io** | Free 10 ülke | — | — | |
| **smspool.net** | Free (2026 testinde başarısız) | Marketplace | ✅ | Bkz. §3 |
| **esimplus.me** | Free SMS | eSIM planları | — | |
| **textr.com** | Free US/UK/CA numara | — | — | Aylık yenilenen havuz |
| **supercloudsms.com** | Free 128 ülke | — | — | 1–5 dk refresh |
| **free-sms-receive.com** | Free US/CA/UK/AU/CN/HK | — | — | Haftalık güncelleme |
| **sms-receive-online.com** | Free 128 ülke | — | — | |
| **valar-sms.com** | — | Geçici numara | — | |
| **smsbus / sms-bus.com** | — | API | ✅ | Bkz. §3 |

---

## 3. TİP C — Ücretli / Freemium Doğrulama API'leri (private inbox)

Bunlar **public inbox değil**; numara satın alırsın, OTP özel gelir. Otomasyon için doğru katman bu.

### 3.1 REST API desenleri

| Servis | Base URL | Auth | Ana endpoint'ler | Rate limit |
|---|---|---|---|---|
| **SMS-Activate** | `https://api.sms-activate.es/stubs/handler_api.php` | `api_key` query param | `getNumber`, `getStatus`, `getFullSms`, `getMultiServiceNumber`, `getRentNumber`, `getRentStatus`, `getPrices`, `getCountries`, `getOperators`, `getActiveActivations` | — |
| **SMS-Activate (dev)** | `https://api.sms-activate.dev` | `Authorization` header | `GET /api/v1/balance`, `GET /api/v1/numbers/available`, `POST /api/v1/numbers/buy`, `GET /api/v1/activations/:id`, `DELETE /api/v1/activations/:id` | — |
| **SMS-Activate (app)** | `https://api.sms-activate.app/v1` | `Authorization: Bearer` | `GET/POST /me/numbers`, `GET /me/sms`, `GET /public/activations/catalog`, `POST /public/activations/order` | — |
| **5sim** | `https://5sim.net/v1` | `Authorization: Bearer <JWT>` | `GET /user/buy/activation/{country}/{operator}/{product}`, guest free: `/v1/guest/free` | — |
| **SMSPool** | `https://api.smspool.net` | `key` (POST form) | `POST /pool/retrieve_valid`, `POST /purchase/sms`, `POST /sms/activate`, `GET /token/{id}` | — |
| **Temp Number** | `https://api.temp-number.com/v1` | `Authorization: Bearer` | `GET /numbers`, `GET /numbers/{id}/sms`, `GET /receive-sms?phoneNumber=`, `POST /numbers/buy`, `/renew`, `/cancel`, `GET /pricing/...` | 429 + `Retry-After`; Idempotency-Key (24h) |
| **iSMScode** | `https://ismscode.com/api/v3` | `Authorization: Bearer` | `POST /numbers/request`, `GET /numbers/{id}/sms`, `GET /services` | **60 req/dk** / API key |
| **SMSZ** | `https://www.smsz.net/api/v1` | `Authorization: Bearer` | `POST /activations`, `GET /activations/{id}/messages`, `POST /rentals`, `GET /pricing/activations`, `POST /webhooks/endpoints` | Retry-After on 429; webhook öneriliyor |
| **SMSTwins** | `https://smstwins.com/v1` | `Authorization: ApiKey sk_live_...` | `POST /activations`, `GET /activations/{id}`, `/reactivate`, `POST /rentals`, `GET /simple?action=...`, `PUT /webhooks` | FREE 60/dk, STANDARD 120/dk, ENTERPRISE 600/dk |
| **SMS-BUS** | `https://sms-bus.com/api/control` | `token` query param | `GET /get/number`, `GET /get/sms`, `GET /reuse` | — |
| **Proxnum** | (REST) | `Authorization: Bearer` | `api/v1/resell/price`, `resell/virtual/buy`, `resell/virtual/{id}/status` | — |
| **Onlinesim** | `https://onlinesim.io` | API key | `getFreeList` (free numaralar + mesajları), kiralama + aktivasyon uçları | — |

### 3.2 Ortak akış

```
1. GET  pricing/catalog        → ülke + servis + stok + fiyat
2. POST activations/buy        → numara tahsis, bakiye düşer
3. GET  .../{id}/messages      → 3–5 sn poll VEYA webhook
4. POST .../{id}/finish|cancel → bitti / iade
```

- Kimlik: çoğu `Bearer` token; SMSPool `key`, SMS-BUS `token`, SMS-Activate `api_key`.
- Webhook imzası: SMSTwins HMAC-SHA256, SMSZ `whsec_` secret.
- Idempotency: Temp Number + SMSZ `Idempotency-Key` (UUID) destekliyor.

### 3.3 Ücretsiz / Kısmi Erişimli API'ler

| Servis | Endpoint | Auth | Dönen |
|---|---|---|---|
| **Onlinesim free list** | `GET https://onlinesim.io/api/getFreeList?country={country}&number={number}` | Free endpoint için API key şartı **tespit edilemedi** | JSON: ülkeler, free numaralar, o numaralara gelen mesajlar, kısıtlı servisler |
| **5sim guest** | `GET https://5sim.net/v1/guest/free` | Yok | Free numara + mesaj listesi |
| **quackr.io** | `https://quackr.io/api` | Profil API key; **yalnız 6/12 aylık kiralama aboneleri** | JSON, sınırsız SMS API erişimi (abonelik şartlı) |
| **anonymsms.com** | `https://api.anonymsms.com/api/documentation` | Doğrulanmadı | Doğrulanmadı |
| **freephonenum generator** | `GET https://ch.freephonenum.com/fake-phone-number-generator/api` | Yok | ⚠️ Üretilen test numaraları — **SMS alamaz**, sadece form doldurmak için |

**Onlinesim `getFreeList` örnek yanıtı:**
```json
{
  "response": 1,
  "countries": [{ "country": 7, "country_text": "Россия", "country_original": "russia" }],
  "numbers": {
    "9915584911": {
      "country": 7, "country_original": "russia",
      "data_humans": "6 дней назад",
      "full_number": "+79915584911", "is_archive": false
    }
  },
  "messages": {
    "current_page": 1,
    "data": [{ "text": "...", "in_number": "***2601", "my_number": 9915584911, "created_at": "2022-07-24 15:32:14" }],
    "per_page": 10, "total": 6100, "last_page": 610, "number": "9915584911", "country": 7
  }
}
```

---

## 4. Açık Kaynak Scraper / Aggregator Projeleri

Public inbox sitelerini programatik okuyan projeler — **otomasyon motorun için asıl referans bunlar.**

| Proje | Dil | Ne yapar | Kapsanan servisler |
|---|---|---|---|
| **Shelex/free-otp-api** | TypeScript | Puppeteer ile sayfa parse + REST API üstü. Redis cache, 10 dk'da browser restart, 15 anon sayfa limiti | receive-sms-free.cc, anonymsms.com, quackr.io, smstome.com, receivesms.co, receiveasmsonline.com, receive-smss-online.com |
| **cvcvka5/pyvirtualsms** | Python | Provider-agnostic scraper kütüphanesi. `requests` + `selectolax`. `.wait_for_message(timeout, interval)` | sms24.me, receive-smss.com, freephonenum.com |
| **oddmario gist** (blocklist scraper) | Python | Çok siteli numara toplayıcı (blocklist üretmek için). Threaded | ~40 site: receive-sms-online.info, receive-smss.com, sms24.me, receivesms.co, receive-sms.cc, sms-receive.net, getfreesmsnumber.com, sms-online.co, freephonenum.com, smstools.online, 5sim guest, 7sim.net, sellaite, hs3x.com, yinsiduanxin, online-sms.org ... |
| **iP1SMS/disposable-phone-numbers** | CSV | `source.csv` — disposable numara kaynak URL listesi | ~60 URL (receive-sms-free.cc ülke sayfaları dahil) |
| **imheheda/awesome-sms-verification** | Markdown | 21 dilde curated liste, fiyat/ülke/ödeme/API karşılaştırması | SMS-Activate, 5sim, TextVerified, jiema.my |
| **therealelyayo/awesomw-sms** | Markdown | Free SMS sağlayıcı listesi | ~40 site |
| **workwayfi/sms-jiema** | Markdown | "1000 ücretsiz yabancı numara" — 128 ülke, 32 platform | Tiger SMS, 超级云短信, ReceiveASMS, AnonymSMS, SuperCloudSMS, SMSnator, ... |
| **SMSRoute-cc/awesome-sms-privacy** | Markdown | No-KYC SMS API + gizlilik araçları | SMSRoute, Twilio, Vonage, Plivo |
| **serhat961/verifysms-landing** (`awesome-sms-verification.md`) | Markdown | Ücretli/ücretsiz/kullanım amacına göre sınıflandırma | — |
| **agentsimdev/awesome-ai-agent-verification** | Markdown | AI ajanları için MCP destekli doğrulama sağlayıcıları | AgentSIM, VoidMob, JoltSMS, GetSMSCode, 5SIM, SMSPool, VirtualSMS |
| **transitive-bullshit/sms-number-verifier** | JavaScript | Otomatik sistemler için SMS doğrulama (215 yıldız) | — |

### 4.2 İkinci Tur Taramada Bulunan Ek Açık Kaynak Projeler

| Proje | Dil | Ne yapar |
|---|---|---|
| **Molx32/SMSExplorer** | Python, JS, HTML, CSS | Public SMS receiver'ları parse edip toplar; pasif yerel DB koleksiyonu |
| **gnh1201/vNumbers** | C# | Büyük ölçekli sanal numara gateway'i; multithreaded gelen SMS crawler |
| **fictus/LibreSMS** | C# (.NET MAUI) | MIT lisanslı self-hosted Android SMS/MMS gateway; abonelik/3. parti yok |
| **we-digital/android-nomad-gateway** | Kotlin/Android | Gelen SMS'i JSON HTTP POST olarak seçilen URL'ye forward eder |
| **beautifulSoup/get-sms** | TypeScript/Node.js | Self-hosted tek-kullanıcı; forward edilen SMS'i SQLite'ta tutar, **MCP tool** açar |
| **LayorX/Temporary-SMS-Receiver-Monitor** | Python | freereceivesms.com, temp-number.com, receive-smss.com public sayfalarını scrape eder, izler |
| **rossigee/sms2webhook** | (doğrulanmadı) | Kendi SIM'inden Android→webhook forward |
| **SunilDhaker/SMSForwarder** | (doğrulanmadı) | Kendi Android cihazından forward |
| **BoostBlitz/smsreceivefree-telegram-bot** | (doğrulanmadı) | Disposable SMS inbox'a erişen Telegram botu |
| **baochouu08/python-receive-sms-online-api** | Python | Gayriresmi Python SMS sitesi scraper/API client |

---

## 5. TİP D — Self-Hosted SMS Gateway (kendi donanımın)

Kodlama/otomasyon motoru için en sürdürülebilir katman: kendi SIM'ini API'ye çevir. Maliyet = operatör planı, per-message ücret yok.

| Proje | Dil / Stack | Ne yapar | API |
|---|---|---|---|
| **capcom6/android-sms-gateway** | Kotlin (5K ★) | Android'i SMS gateway yapar. Local server (cihazda) veya cloud server. E2E şifreleme. Webhook | `POST /3rdparty/v1/message`, `POST /3rdparty/v1/webhooks`, CLI `smsgate` |
| **textbee/textbee** | React, Next.js, NestJS, MongoDB, Kotlin | Android SMS gateway. REST + dashboard + **MCP server** (Claude/Cursor) | `POST /api/v1/gateway/send-sms`, `GET /api/v1/gateway/messages?direction=received`, `list_devices`. Header: `x-api-key` |
| **NdoleStudio/httpsms** | Go | Android'i SMS gateway yapar. Webhook'a forward | HTTP API (Go client), `httpsms.com` cloud veya self-host |
| **smskit/smskit** | PHP + Java (Android) | Flat-file JSON backend + Android app. Telefon 5 sn'de bir poll eder, gelen cevapları dashboard'a iletir | `POST /api/v1/send.php`, `GET /status.php`, `/statistics.php`, `/validate.php`, `/schedule.php`. Bearer token |
| **MahmoudY3c/react-native-sms-gateway** | React Native (Kotlin) | Arka plan SMS listener → HTTP/Telegram'a forward. Boot persistence | JS event emitter + HTTP endpoint + Telegram bot |
| **SMSTwins provider gateway** | WebSocket | Kendi SIM donanımını marketplace'e sat (sağlayıcı tarafı) | `wss://smstwins.com/gateway`, `auth`/`numbers_sync`/`sms_received` event'leri, device token |

---

## 6. Doğrulama / Ölçüm Gerçekleri (2026)

- **Public inbox = sıfır gizlilik.** Kod gelse bile aynı sayfayı izleyen başkası kapabilir.
- **VoIP tespiti:** Major servisler numaranın VoIP mi gerçek mobil mi olduğunu kontrol ediyor; VoIP reddediliyor.
- **Numara yakma hızı:** Her numaraya günde binlerce doğrulama denemesi → saatler içinde bloklanıyor.
- **Canlı test sonucu (Pixel Defence, 12 adaydan):** 5'i ölü/başarısız (SMSPool Free, Receive-SMSS, GetFreeSMS, OnlineSIM, SMSReceiveFree); 7'si teslim etti (Receive-SMS.app ~18sn, FreePhoneNum ~22sn, SMSToMe ~25sn, SMS-Online.co ~30sn, ReceiveSMS.cc ~32sn, Temp-Number.org ~35sn, 7SIM ~40sn).
- **sms24.me canlı sayıları:** 53 ülke, 10.498 public inbox (2026-09-28 çekildi).
- **AnonymSMS en çok SMS alan numaralar:** +447884641162 (117.365 SMS), +4915210229762 (70.348), +4915210336958 (63.430).

---

## 7. Motor İçin Öneri

1. **Public inbox siteleri (Tip A)** — kalıcı çözüm değil. Ancak ucuz/free fallback olarak `pyvirtualsms` / `free-otp-api` / `SMSExplorer` deseniyle scrape edilebilir. İlk üç aday: **sms24.me** (53 ülke, 10.498 inbox), **receive-smss.com** (50+ ülke), **freephonenum.com** (US/CA).
2. **Freemium API (Tip B/C)** — `onlinesim getFreeList`, `5sim /v1/guest/free` gibi guest endpoint'ler kayıtsız programatik erişim veriyor. Otomasyonda ilk denenecek katman.
3. **Self-hosted gateway (Tip D)** — sürdürülebilir, 0 per-message maliyet. `capcom6/android-sms-gateway` (5K★, E2E, local+cloud), `textbee` (MCP server'lı) veya `LibreSMS` (MIT, .NET MAUI) ile kendi SIM'ini API'ye çevir. Uzun vadeli doğru mimari bu.
4. **Scraper referansları** — `Shelex/free-otp-api` (Puppeteer + Redis + REST), `cvcvka5/pyvirtualsms` (provider-agnostic Python) ve `LayorX/Temporary-SMS-Receiver-Monitor` (canlı izleme) doğrudan kod iskeleti olarak alınabilir.

### Özet sayılar (2026-09-28)

| Katman | Adet |
|---|---|
| Public inbox sitesi (Tip A) | **121** (20'si canlı çekildi) |
| Freemium sağlayıcı (Tip B) | 20 |
| Ücretli/API doğrulama (Tip C) | 12 servis, 5 dokümante API deseni |
| Açık kaynak proje | **24** (14 katalog + 10 ek) |
| Self-hosted gateway (Tip D) | 6 |

---

## Kaynakça

- sms24.me, receive-sms-free.cc, quackr.io, sms-online.co, anonymsms.com, receivesms.co, receive-sms-online.info, receivesmsonline.net, freephonenum.com, receive-smss.com, freeonlinephone.org, sms.sellaite.com, hs3x.com, 7sim.net, receive-a-sms.com, sms-receive.net, pingme.tel, online-sms.org, smstome.com, smstools.online — canlı sayfa çekimleri (2026-09-28)
- onlinesim.io/openapi_docs, temp-number.com/developers, ismscode.com/api-doc, smsz.net/api/llms-full.txt, smstwins.com/docs, sms-bus.com/docs, proxnum.com/sms-verification-api, 5sim.net/docs, sms-activate.dev/api-docs, sms-activate.es/api2 — API dokümanları
- github.com/Shelex/free-otp-api, cvcvka5/pyvirtualsms, iP1SMS/disposable-phone-numbers, imheheda/awesome-sms-verification, workwayfi/sms-jiema, SMSRoute-cc/awesome-sms-privacy, agentsimdev/awesome-ai-agent-verification
- github.com/capcom6/android-sms-gateway, textbee/textbee, NdoleStudio/httpsms, smskit/smskit, MahmoudY3c/react-native-sms-gateway
- blog.makeinfo.co, geckoandfly.com, techmende.medium.com, technowizah.com, solutionself.com, blog.dingtone.me, sms-activate.com/blog, nadanada.me/blog, pixeldefence.com — karşılaştırma listeleri

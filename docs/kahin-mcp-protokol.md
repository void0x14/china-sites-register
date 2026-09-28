# Kahin MCP — Go'dan Sürme Protokolü

Belge amacı: bir Go programının Kahin MCP sunucusunu nasıl başlatıp konuşacağını
tahminsiz, kaynak satır referansı ve gerçek komut çıktısıyla sabitlemek.

Tüm komutlar bu makinede gerçekten çalıştırıldı. Çalıştırılamayan hiçbir iddia
"doğrulandı" diye yazılmadı; doğrulanamayanlar açıkça işaretlendi.

---

## 0. Özet (tek ekran)

| Soru | Kesin cevap |
|---|---|
| Transport | **stdio** (newline-delimited JSON). SSE/streamable-HTTP yok. |
| Çerçeveleme | **`\n` ile ayrılmış JSON** — Content-Length **DEĞİL** |
| Kodlama | UTF-8 |
| Protokol versiyonu | İstemci ne isterse; destekleniyorsa aynen yansıtılır, değilse `2025-11-25` |
| Tool sonucu | `content[0].text` = JSON **metni**; `structuredContent.result` = **aynı metin** |
| Go'da sürmek | `os/exec` + `StdinPipe` + `StdoutPipe`, satır satır JSON encode/decode |
| Zorunlu ortam değişkeni | **Yok**. `cwd` nötr bir dizin olmalı (bkz. §6.5, §6.6). |
| En kritik tuzak | Makine geneli **tek** tarayıcı slotu — ikinci MCP süreci `engine_process_conflict` alır (§6.3) |
| Kanıtlanan canlı akış | `start → navigate → health → snapshot → screenshot → stop` (hepsi Go'dan) |

---

## 1. Transport: stdio

### Kanıt — kaynak satırı

`kahin/oracle.py:117-118` (tek `mcp.run` çağrısı, tüm repoda):

```python
def main() -> None:
    mcp.run(transport="stdio")
```

Doğrulama:

```bash
$ grep -rn "mcp.run\|\.run(transport" kahin/ --include=*.py
kahin/oracle.py:118:    mcp.run(transport="stdio")
```

`kahin/tools/*.py` içinde SSE/streamable-http/uvicorn/starlette kurulumu **yok**:

```bash
$ grep -rn "run_sse\|streamable_http\|sse_app\|streamable_http_app\|uvicorn\|starlette" kahin/ --include=*.py
(bos = yok)
```

`FastMCP.run` varsayılanı da `stdio`, ama Kahin zaten açıkça `stdio` veriyor
(`mcp/server/fastmcp/server.py`):

```python
def run(self, transport: Literal["stdio", "sse", "streamable-http"] = "stdio", ...)
```

**Sonuç:** `python -m kahin.oracle` **stdio** transport ile konuşur. SSE veya
streamable HTTP **kullanılmaz** — bu transportlara geçmenin bir yolu da yoktur
(kod `mcp.run`'a argümanı sabit geçer).

### Kanıt — çerçeveleme newline-delimited

`mcp/server/stdio.py:47,49` (UTF-8 metin sarmalama):

```python
stdin  = anyio.wrap_file(TextIOWrapper(sys.stdin.buffer,  encoding="utf-8", errors="replace"))
stdout = anyio.wrap_file(TextIOWrapper(sys.stdout.buffer, encoding="utf-8"))
```

`mcp/server/stdio.py:63,65` (satır satır okuma):

```python
async for line in stdin:
    message = types.JSONRPCMessage.model_validate_json(line)
```

`mcp/server/stdio.py:80` (satır + `\n` ile yazma):

```python
await stdout.write(json + "\n")
```

Canlı ham bayt kanıtı (Content-Length **yok**, satır sonu **var**):

```
=== C) stdout ham bayt ===
ilk 60 bayt: b'{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06'
son 5 bayt : b'."}}\n'
Content-Length iceriyor mu: False
newline ile bitiyor mu: True
```

**Karşı-test (negatif kanıt):** Content-Length framed mesaj gönderildiğinde
sunucu mesajı parse edemez ve hata bildirimi basar:

```
=== A) Content-Length framing denemesi ===
stdout: '{"method":"notifications/message","params":{"level":"error",
         "logger":"mcp.server.exception_handler","data":"Internal Server Error"},"jsonrpc":"2.0"}\n...'
stderr: ... [type=json_invalid, input_value='\\n', input_type=str] ...
```

Yani LSP tarzı `Content-Length: N\r\n\r\n{...}` çerçeveleme **kullanılmaz ve
çalışmaz**. Go tarafında her mesaj tek satır JSON olarak yazılmalı, `\n` ile
bitirilmeli.

---

## 2. Handshake akışı

Sıra: `initialize` → `notifications/initialized` → `tools/list` → `tools/call`.

### 2.1 Protokol versiyonu

SDK sabitleri (`mcp/types.py:27,35` ve `mcp/shared/version.py`):

```python
LATEST_PROTOCOL_VERSION = "2025-11-25"
DEFAULT_NEGOTIATED_VERSION = "2025-03-26"
SUPPORTED_PROTOCOL_VERSIONS = ["2024-11-05", "2025-03-26", "2025-06-18", LATEST_PROTOCOL_VERSION]
```

Yansıtma kuralı (`mcp/server/session.py:178-187`):

```python
requested_version = params.protocolVersion
...
protocolVersion = requested_version if requested_version in SUPPORTED_PROTOCOL_VERSIONS
                  else types.LATEST_PROTOCOL_VERSION
```

**Gerçek ölçüm matrisi** (her versiyon ayrı süreçte denendi):

```
requested=2024-11-05   -> negotiated=2024-11-05  server={'name': 'kahin', 'version': '0.3.10'}
requested=2025-03-26   -> negotiated=2025-03-26  server={'name': 'kahin', 'version': '0.3.10'}
requested=2025-06-18   -> negotiated=2025-06-18  server={'name': 'kahin', 'version': '0.3.10'}
requested=2025-11-25   -> negotiated=2025-11-25  server={'name': 'kahin', 'version': '0.3.10'}
requested=1999-01-01   -> negotiated=2025-11-25  server={'name': 'kahin', 'version': '0.3.10'}
```

Yani Kahin **kendi versiyonunu empoze etmez**; istemcinin istediği sürümü
destekliyorsa onu onaylar. Go tarafı `2025-06-18` isterse onu alır.

Sunucu kimliği: `name="kahin"`, `version="0.3.10"`.

`serverInfo.version`, `kahin/_mcp.py:64` ile paket sürümüne sabitlenmiştir:

```python
mcp._mcp_server.version = __version__   # kahin/__init__.py:23 → __version__ = "0.3.10"
```

### 2.2 `initialized` bildirimi ne kadar zorunlu?

SDK, initialize edilmeden gelen **istekleri** reddeder
(`mcp/server/session.py:204-205`):

```python
case _:
    if self._initialization_state != InitializationState.Initialized:
        raise RuntimeError("Received request before initialization was complete")
```

Ancak `initialize` yanıtı verildiği an durum zaten `Initialized` olur
(`mcp/server/session.py:199`), yani `initialized` bildirimi gelmese de
`tools/list` çalışır. **Ölçüm:**

```
=== B) initialized gonderilmeden tools/list ===
error var mi: False | tool sayisi: 161
initialized SONRASI tools/list tool sayisi: 161

=== D) initialized GONDERILMEDEN tools/call ===
{"jsonrpc":"2.0","id":2,"result":{...engine_unavailable...},"isError":false}
```

**Ama `initialize` atlanırsa çalışmaz:**

```
=== E) initialize ATLAMADAN tools/list ===
{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"Invalid request parameters","data":""}}
```

**Pratik kural:** `initialize` **zorunlu**, `notifications/initialized` **toleranslı
ama gönderilmeli** (spec uyumu; başka MCP SDK'ları istemezse reddedebilir).
Go taslağı ikisini de gönderiyor.

---

## 3. JSON-RPC mesaj örnekleri

Hepsi gerçek şemalardan üretildi; `tools/list` çıktısındaki `inputSchema`
alanlarından birebir alındı.

### 3.1 initialize (Go → Kahin)

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"kahin-mcp-probe","version":"0.1.0"}}}
```

Yanıt (gerçek, kırpılmış):

```json
{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","capabilities":{"experimental":{},"prompts":{"listChanged":false},"resources":{"subscribe":false,"listChanged":false},"tools":{"listChanged":false}},"serverInfo":{"name":"kahin","version":"0.3.10"},"instructions":"I am the Oracle. ..."}}
```

### 3.2 initialized (Go → Kahin, id YOK)

```json
{"jsonrpc":"2.0","method":"notifications/initialized"}
```

### 3.3 tools/list

```json
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
```

Yanıt: `result.tools` dizisi. Repo `.venv` ile **161 tool**, kurulu venv ile
**146 tool** (bkz. §6.3 fark).

### 3.4 tools/call — gerçek argüman şemaları

Aşağıdaki şemalar `tools/list` yanıtından birebir alındı:

| Tool | Gerekli alan | Şema (özet) |
|---|---|---|
| `kahin_browser_start` | — | `engine`(="mirage"), `headless`(=true), `port`(=0), `identity`, `proxy`, `persistent_profile`(=true), `profile_dir`, `addons`, `passkey_mode`(=false), `mode`, `ephemeral_ack`(=false) |
| `kahin_navigate` | `url` | `url`, `wait_until`(="load"), `timeout`(=30.0), `referer` |
| `kahin_mirage_snapshot` | — | `selector`, `max_tokens`(=1500), `include_hidden`(=false), `frame_id` |
| `kahin_mirage_click` | `selector` | `selector`, `timeout`(=10.0), `return_snapshot`(=false), `frame_id` |
| `kahin_mirage_fill_form` | `fields` | `fields`(obje dizisi), `timeout`(=10.0), `frame_id` |
| `kahin_mirage_dom_action` | `node_id`, `action` | `node_id`, `action`, `text`, `frame_id` |
| `kahin_screenshot` | — | `full_page`(=false) |
| `kahin_engine_health` | — | (parametresiz) |
| `kahin_mirage_dom_snapshot` | — | `selector`, `max_nodes`(=800), `max_depth`(=12), `include_hidden`(=false), `text_limit`(=240), `frame_id` |

Ham şema örnekleri:

```json
{"properties":{"url":{"title":"Url","type":"string"},"wait_until":{"default":"load","title":"Wait Until","type":"string"},"timeout":{"default":30.0,"title":"Timeout","type":"number"},"referer":{"anyOf":[{"type":"string"},{"type":"null"}],"default":null,"title":"Referer"}},"required":["url"],"title":"navigateArguments","type":"object"}
```

```json
{"properties":{"selector":{"title":"Selector","type":"string"},"timeout":{"default":10.0,"title":"Timeout","type":"number"},"return_snapshot":{"default":false,"title":"Return Snapshot","type":"boolean"},"frame_id":{"anyOf":[{"type":"string"},{"type":"null"}],"default":null,"title":"Frame Id"}},"required":["selector"],"title":"mirage_clickArguments","type":"object"}
```

```json
{"properties":{"node_id":{"title":"Node Id","type":"string"},"action":{"title":"Action","type":"string"},"text":{"anyOf":[{"type":"string"},{"type":"null"}],"default":null,"title":"Text"},"frame_id":{"anyOf":[{"type":"string"},{"type":"null"}],"default":null,"title":"Frame Id"}},"required":["node_id","action"],"title":"mirage_dom_actionArguments","type":"object"}
```

```json
{"properties":{"fields":{"items":{"additionalProperties":true,"type":"object"},"title":"Fields","type":"array"},"timeout":{"default":10.0,"title":"Timeout","type":"number"},"frame_id":{"anyOf":[{"type":"string"},{"type":"null"}],"default":null,"title":"Frame Id"}},"required":["fields"],"title":"mirage_fill_formArguments","type":"object"}
```

```json
{"properties":{"full_page":{"default":false,"title":"Full Page","type":"boolean"}},"title":"screenshotArguments","type":"object"}
```

### 3.5 Örnek çağrı gövdeleri

```json
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"kahin_browser_start","arguments":{"engine":"mirage","headless":true,"mode":"keş","ephemeral_ack":true}}}
```

```json
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"kahin_navigate","arguments":{"url":"https://example.com","wait_until":"load","timeout":30.0}}}
```

```json
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"kahin_mirage_snapshot","arguments":{"max_tokens":1500}}}
```

```json
{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"kahin_mirage_click","arguments":{"selector":"#submit","timeout":10.0}}}
```

```json
{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"kahin_mirage_fill_form","arguments":{"fields":[{"selector":"#phone","value":"5551234"}]}}}
```

> `kahin_mirage_fill_form.fields` şeması `additionalProperties:true` verir; alan
> adları sunucu tarafında çözülür. Kullanılan `selector`/`value` anahtarları
> **doğrulanamadı** (canlı deneme yapılmadı); sunucu içeriğini `kahin_find_concept`
> veya `kahin/tools/agent_mirage.py:301` üzerinden teyit et.

```json
{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"kahin_mirage_dom_action","arguments":{"node_id":"nwd0umnfl-1","action":"click"}}}
```

```json
{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"kahin_screenshot","arguments":{"full_page":false}}}
```

```json
{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"kahin_engine_health","arguments":{}}}
```

```json
{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"kahin_mirage_dom_snapshot","arguments":{"max_nodes":800,"max_depth":12}}}
```

---

## 4. Tool sonuçları nasıl dönüyor?

### Kaynak mekanizması

Kahin tool'larının hepsi Python'da `-> str` döner (ör. `kahin/tools/engine.py:26`,
`kahin/tools/pilot.py:437,1217,1453`), yani **içerik zaten JSON metnidir**.
FastMCP bunu `mcp/server/fastmcp/utilities/func_metadata.py:120-132` ile sarar:

```python
unstructured_content = _convert_to_content(result)   # -> TextContent(text=<str>)
if self.wrap_output:
    result = {"result": result}                       # <- string {"result": "<json metni>"} içine gömülür
validated = self.output_model.model_validate(result)
structured_content = validated.model_dump(mode="json", by_alias=True)
return (unstructured_content, structured_content)
```

`wrap_output` nedeniyle `structuredContent` **düz obje değil**, tek anahtarlı
`{"result": "<JSON metni>"}` zarfıdır. `outputSchema` da bunu bildirir:

```json
"outputSchema": {"properties":{"result":{"title":"Result","type":"string"}},"required":["result"],"title":"engine_healthOutput","type":"object"}
```

### Gerçek yanıt örnekleri

**Başarısız/boş motor (`kahin_engine_health`, motor yok):**

```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"{\n  \"engine\": null,\n  \"error\": \"No browser engine running.\",\n  \"code\": \"engine_unavailable\",\n  \"last_engine_death\": null\n}"}],"structuredContent":{"result":"{\n  \"engine\": null,\n  \"error\": \"No browser engine running.\",\n  \"code\": \"engine_unavailable\",\n  \"last_engine_death\": null\n}"},"isError":false}}
```

**Başarılı bilgi çağrısı (`kahin_get_command`):**

```json
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"{\n  \"name\": \"navigate\",\n  \"domain\": \"Page\",\n  ..."}],"structuredContent":{"result":"{\n  \"name\": \"navigate\",\n  \"domain\": \"Page\",\n  ..."},"isError":false}}
```

**Doğrulama hatası (şema dışı argüman):**

```json
{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"Error executing tool kahin_validate_command: 3 validation errors for validate_commandArguments\ndomain\n  Field required ..."}],"isError":true}}
```

Bu son örnek önemli: **protokol hatası JSON-RPC `error` olarak değil**, `result`
içinde `isError:true` + `content[0].text` olarak döner.

### Go tarafında doğru okuma

Doğrulanan ilişki:

```
sc.result tipi: str
sc.result == content[0].text ? True
```

`structuredContent` bir **obje değil**, `{"result": "<string>"}` zarfıdır. Go'da
önce `.result` string'ini alıp sonra JSON olarak parse etmek gerekir:

```go
type callToolResult struct {
    Content           []struct{ Type, Text string } `json:"content"`
    StructuredContent map[string]any                `json:"structuredContent,omitempty"`
    IsError           bool                          `json:"isError"`
}

func (r *callToolResult) ToolText() string {
    if v, ok := r.StructuredContent["result"].(string); ok && v != "" {
        return v
    }
    for _, b := range r.Content {
        if b.Type == "text" && b.Text != "" { return b.Text }
    }
    return ""
}
```

`structuredContent` alanı **her zaman gelmez** (ör. hata durumunda yok).
Bu yüzden `ToolText()` önce `structuredContent`, sonra `content[0].text` okur.

**Uyarı:** `isError:false` "iş başarılı" demek değildir. Motor yokken de
`isError:false` döndü, ama gövdede `"code":"engine_unavailable"` var. Go tarafı
her tool çıktısını ayrıca `code`/`error` alanları için JSON olarak parse etmeli.

---

## 5. Go'dan sürme — minimal kod

Çalışan taslak: `/tmp/kahin-mcp-probe/main.go` (derlendi, koşuldu).
Yardımcı dosya: `/tmp/kahin-mcp-probe/go.mod` (`module kahinmcpProbe`, `go 1.24`).

### Çekirdek iskelet

```go
cmd := exec.Command(pythonBin, "-m", "kahin.oracle")
cmd.Dir = workDir                  // kahin paketinin import edilebilmesi için
cmd.Env = append(os.Environ(), extraEnv...)

stdin,  _ := cmd.StdinPipe()
stdout, _ := cmd.StdoutPipe()
stderr, _ := cmd.StderrPipe()      // tanılama; protokole KARIŞMAZ
cmd.Start()

rd := bufio.NewReaderSize(stdout, 1<<20)   // satır okuyucu

// her mesaj: json.Marshal(req) + '\n'  -> stdin
// yanıt     : rd.ReadBytes('\n')       -> json.Unmarshal
```

Kritik noktalar (hepsi koda gömülü yorum olarak da yazıldı):

1. **`cmd.Dir` ne olmalı?** Ölçüldü: **her iki venv de yabancı bir `cwd`'den
   çalıştı.**

   ```bash
   $ ./kahin-mcp-probe -python <repo>/.venv/bin/python -dir /tmp/kahin-clean-cwd
   initialize OK: server=kahin v0.3.10 protocolVersion=2025-06-18
   tools/list OK: 161 tool

   $ ./kahin-mcp-probe -python ~/.local/share/kahin/venv/bin/python -dir /tmp/kahin-clean-cwd
   initialize OK: server=kahin v0.3.10 protocolVersion=2025-06-18
   tools/list OK: 146 tool
   ```

   Sebep: repo `.venv` **editable** kurulu (`.venv/lib/python3.13/site-packages/_editable_impl_kahin.pth`),
   `import kahin` repo kaynağını gösterir:

   ```
   $ cd /tmp/kahin-clean-cwd && <repo>/.venv/bin/python -c "import kahin; print(kahin.__file__)"
   /home/void0x14/Documents/mcp-projelerim/cdp-kahin-mcp/kahin/__init__.py
   ```

   Yani `cwd` = **nötr bir dizin** yeterli; repo kökü olmak zorunda değil. Ama
   bkz. §6.5: `/tmp` gibi yabancı `.so` içeren bir dizin **seçilmemeli**.
2. **`cmd.Env = append(os.Environ(), ...)`** — ortamı sıfırdan kurmayın;
   `HOME`/`XDG_*`/`DISPLAY` devralınmalı.
3. **stderr ayrı okunmalı.** Kahin tanılamayı stderr'e yazar; stdout yalnız
   JSON-RPC taşır. Yine de stdout'ta JSON olmayan satır gelirse taslak onu
   atlar ve stderr'e raporlar.
4. **id eşleştirmesi.** Sunucu `notifications/message` gibi id'siz bildirimler
   basabilir. Taslak id'siz satırları atlar, id uyuşmayanları atlar.
5. **`bufio.ReaderSize(stdout, 1<<20)`** — `tools/list` yanıtı büyük (161 tool,
   ~200 KB+). Küçük buffer kullanmayın.

### Kullanım

```bash
cd /tmp/kahin-mcp-probe
go run .                                  # initialize + tools/list
go run . -call kahin_engine_health -args '{}'
go run . -flow -lock /tmp/kahin-probe/browser.lock -home /tmp/kahin-probe/home
```

Bayraklar:

| Bayrak | Varsayılan | Anlam |
|---|---|---|
| `-python` | repo `.venv/bin/python` | Kahin python yolu |
| `-dir` | repo kökü | alt süreç `cwd` |
| `-call` | boş | çağrılacak tool |
| `-args` | `{}` | tool argümanları |
| `-lock` | boş | `KAHIN_BROWSER_LOCK_PATH` override |
| `-home` | boş | `KAHIN_HOME` override |
| `-flow` | `false` | tam tarayıcı akışı |

---

## 6. Ortam değişkenleri / config

### 6.1 Zorunlu mu? Hayır.

`initialize` + `tools/list` **hiçbir ortam değişkeni olmadan** çalıştı (yukarıdaki
tüm testler bunu gösteriyor). Tarayıcı açmak için de zorunlu bir değişken yok —
varsayılanlar makul.

### 6.2 Tam liste (kaynak taraması)

```bash
$ grep -rhno "KAHIN_[A-Z_]*\|OBSCURA_PATH\|XDG_[A-Z_]*" kahin/*.py kahin/tools/*.py kahin/the_twins/*.py kahin/residual_self/*.py | sort -u
KAHIN_BROWSER_LOCK_PATH
KAHIN_CAMOUFOX_BIN
KAHIN_DISABLE_COOP
KAHIN_EXTENSION_DIR
KAHIN_FATE_PATH
KAHIN_FORCE_SOFTWARE_GL
KAHIN_HOME
KAHIN_HUMANIZE
KAHIN_OBSCURA_DIR
KAHIN_PROFILE_CACHE_DIR
KAHIN_PROFILE_DIR
KAHIN_ZIG_CORE
OBSCURA_PATH
XDG_CACHE_HOME
XDG_CONFIG_HOME
XDG_DATA_HOME
XDG_RUNTIME_DIR
```

Önemli olanlar:

| Değişken | Kaynak satır | Etki |
|---|---|---|
| `KAHIN_BROWSER_LOCK_PATH` | `kahin/_state.py:59` | Makine geneli tarayıcı slotu kilidinin yeri |
| `KAHIN_HOME` | `kahin/the_twins/mirage.py:125-136`, `kahin/bitwarden.py:39` | Profil/kalıcı veri kökü. Boşsa `~/.local/share/kahin` |
| `KAHIN_PROFILE_DIR` | `kahin/the_twins/mirage.py:91` | `ağırbaş` profil dizini override (mutlak yol) |
| `KAHIN_FATE_PATH` | `kahin/residual_self/fate.py:17` | CDP desen belleği. Boşsa `$XDG_CONFIG_HOME/kahin/fate_db.json` |
| `KAHIN_ZIG_CORE` | `kahin/the_twins/mirage.py:356` | Sidecar binary override |
| `KAHIN_CAMOUFOX_BIN` | `kahin/the_twins/mirage.py:532` | Camoufox binary override |
| `KAHIN_FORCE_SOFTWARE_GL` | `kahin/the_twins/mirage.py:873` | `LIBGL_ALWAYS_SOFTWARE=1` |
| `KAHIN_HUMANIZE` / `KAHIN_DISABLE_COOP` | `kahin/stealth.py:104,142` | Deneysel stealth bayrakları (opt-in) |

`XDG_RUNTIME_DIR` boşsa kilit `/tmp/kahin/browser.lock` olur
(`kahin/_state.py:63`).

### 6.3 ⚠ Makine geneli tek tarayıcı slotu — en kritik operasyonel kısıt

`kahin/_state.py:48` (`acquire_browser_lock`), `flock(LOCK_EX|LOCK_NB)` ile
**makine geneli tek** tarayıcı sahipliği alır. İkinci bir MCP süreci aynı slotu
alamaz. Gerçek çıktı:

```json
{
  "error": "Another Kahin MCP process owns the machine-wide browser slot; refusing to open a second browser.",
  "code": "engine_process_conflict",
  "tool": "kahin_browser_start",
  "hint": "Reuse the owner MCP session or stop it before starting a new session.",
  "path": "/run/user/1000/kahin/browser.lock",
  "owner": {"pid": 3334529, "path": "/run/user/1000/kahin/browser.lock"}
}
```

Kilit sahibinin kim olduğu ölçüldü:

```bash
$ cat /run/user/1000/kahin/browser.lock
{"pid": 3334529, "path": "/run/user/1000/kahin/browser.lock"}

$ ps -p 3334529 -o pid,ppid,cmd
    PID    PPID CMD
3334529 3327233 /home/void0x14/Documents/mcp-projelerim/cdp-kahin-mcp/.venv/bin/python -m kahin.oracle

$ ps -p 3327233 -o pid,cmd
    PID CMD
3327233 grok

$ ls -l /proc/3334529/fd | grep lock
7 -> /run/user/1000/kahin/browser.lock
```

Yani kilidi **bu Grok oturumunun kendi Kahin MCP sunucusu** tutuyor.

**Go programı için iki seçenek:**

1. **Aynı slotu paylaş** — mevcut MCP oturumu durdurulmalı. Basit ama çakışır.
2. **Ayrı slot** — `KAHIN_BROWSER_LOCK_PATH` + `KAHIN_HOME` ayrı verilir.
   Taslakta `-lock` / `-home` bayrakları bunu yapar. Aşağıdaki canlı akış bu
   yöntemle koşuldu ve **çalıştı**.

### 6.4 İki venv farkı — hangisini kullanmalı?

Ölçüldü:

| Venv | Python | `mcp` | Tool sayısı | Konum |
|---|---|---|---|---|
| Repo `.venv` | 3.13 | 1.29.0 | **161** | `cdp-kahin-mcp/.venv` |
| Kurulu venv | 3.14 | 1.30.0 | **146** | `~/.local/share/kahin/venv` |

Fark (15 tool, hepsi **repo `.venv`'de var, kurulu venv'de yok**):

```
kahin_cf_clear, kahin_cf_status, kahin_crawl_events, kahin_extension_prepare,
kahin_mirage_clear_overlays, kahin_mirage_eval, kahin_mirage_watch_start,
kahin_mirage_watch_stop, kahin_ocr, kahin_passkey_setup_close,
kahin_passkey_setup_finish, kahin_passkey_setup_open, kahin_passkey_setup_status,
kahin_vault_login, kahin_visualize_data
```

Kurulu venv'de **fazladan** tool yok. Yani kurulu venv 0.3.10'un daha eski bir
derlemesidir (Eyl 23), repo ise daha güncel (Eyl 28).

**Karar:** İstenen 9 tool **her iki venv'de de var**. 15 ek tool gerekiyorsa
repo `.venv` kullanılmalı. İkisi de `initialize`+`tools/list`+`tools/call`
akışını birebir aynı protokolle konuşuyor — protokol açısından fark yok.

### 6.5 ⚠ Kurulu venv + `/tmp` cwd tuzağı

Kurulu venv ile `cwd=/tmp` kullanıldığında süreç **çöktü**:

```
initialize failed: read initialize: EOF (stderr: [Traceback ... ImportError:
/tmp/zlib.so: wrong ELF class: ELFCLASS32])
```

Sebep: `/tmp` altında artık 32-bit `zlib.so` var; `cwd` sys.path'e girince
Python 3.14 stdlib `zlib` import'u o dosyayı buluyor. Temiz bir `cwd` ile aynı
venv sorunsuz çalıştı:

```bash
$ cd /tmp/kahin-clean-cwd && go run . -python ~/.local/share/kahin/venv/bin/python -dir /tmp/kahin-clean-cwd
initialize OK: server=kahin v0.3.10 protocolVersion=2025-06-18
tools/list OK: 146 tool
```

**Kural:** alt süreç `cwd`'si olarak `/tmp` veya yabancı `.so`/`.py` içeren bir
dizin **kullanılmamalı**. Nötr bir dizin veya repo kökü seçin.

### 6.6 ⚠ İki yol `cwd`'ye ve kurulum şekline bağlı

**(a) Ekran görüntüsü `cwd`'ye göre yazılır** — `kahin/tools/pilot.py:1484`:

```python
out_dir = _P("screenshots")   # GÖRECELİ yol → <cwd>/screenshots/
out_dir.mkdir(parents=True, exist_ok=True)
fpath = out_dir / fname
```

Yani `cmd.Dir` ne ise PNG oraya yazılır. Ölçülen örnek (cwd = repo kökü):

```json
{"path": "/home/void0x14/Documents/mcp-projelerim/cdp-kahin-mcp/screenshots/screenshot-1790627846017.png",
 "format": "png", "bytes": 22050}
```

Go tarafı için: `cmd.Dir` = ayrılmış bir çalışma dizini verin, sonra mutlak
`path` alanını okuyun. `screenshots/` alt dizini sizin dizininizde oluşur.

**(b) Sidecar stderr log'u paket konumuna göre yazılır** —
`kahin/the_twins/mirage.py:934`:

```python
log_dir = Path(__file__).resolve().parents[2] / "logs"
```

Ölçülen iki farklı sonuç:

| Kurulum | `mirage.py` | `log_dir` |
|---|---|---|
| repo `.venv` (editable) | `<repo>/kahin/the_twins/mirage.py` | `<repo>/logs` ✔ |
| kurulu venv | `~/.local/share/kahin/venv/lib/python3.14/site-packages/kahin/the_twins/mirage.py` | `~/.local/share/kahin/venv/lib/python3.14/site-packages/logs` |

Kurulu venv'de `parents[2]` → `site-packages`, yani log'lar **site-packages
içine** düşer. Bu davranış `kahin_engine_health` yanıtındaki `stderr_log`
alanında görünür:

```
repo .venv:    ".../cdp-kahin-mcp/logs/kahin-sidecar-3372540-....err"
kurulu venv:   ".../site-packages/logs/kahin-sidecar-....err"
```

Go tarafı log yolunu **tahmin etmemeli**; `kahin_engine_health` yanıtındaki
`stderr_log` alanını okumali.

**(c) Kalıcı profil yeri `KAHIN_HOME`'a bağlı** — `kahin/the_twins/mirage.py:125-136`:

```python
configured = os.environ.get("KAHIN_HOME", "").strip()
if configured: return Path(configured).expanduser()
data_home = os.environ.get("XDG_DATA_HOME", "").strip()
root = Path(data_home).expanduser() if data_home else Path.home() / ".local" / "share"
return root / "kahin"
```

Ölçülen: `KAHIN_HOME=/tmp/kahin-probe/home` verildiğinde geçici `keş` profili
`/tmp/kahin-probe/home/kes/kahin-kes-...` altında oluştu. `KAHIN_HOME`
verilmezse `~/.local/share/kahin` kullanılır — bu, bu makinede mevcut Kahin
oturumunun profilidir, paylaşılır.

---

## 7. Test — Go taslağının gerçek koşu çıktısı

### 7.1 `go build`

```bash
$ cd /tmp/kahin-mcp-probe
$ export GOFLAGS=-mod=mod GOCACHE=/tmp/gocache GOPATH=/tmp/gopath
$ go build -o kahin-mcp-probe .
$ ls -la kahin-mcp-probe
-rwxr-xr-x 1 void0x14 void0x14 3631538 kahin-mcp-probe
```

Derleme sırasında bir hata çıktı ve düzeltildi (`cannot indirect stderrLines`);
sonrasında temiz derlendi. Go sürümü: `go1.26.5`.

### 7.2 `go run` — initialize + tools/list

```bash
$ go run .
initialize OK: server=kahin v0.3.10 protocolVersion=2025-06-18
tools/list OK: 161 tool
  kahin_browser_start          present=true
  kahin_navigate               present=true
  kahin_mirage_snapshot        present=true
  kahin_mirage_click           present=true
  kahin_mirage_fill_form       present=true
  kahin_mirage_dom_action      present=true
  kahin_screenshot             present=true
  kahin_engine_health          present=true
  kahin_mirage_dom_snapshot    present=true
```

**Kanıtlanan:** Go'dan gerçek MCP handshake'i yapılıyor; 161 tool listeleniyor;
istenen 9 tool'un hepsi mevcut.

### 7.3 `go run -call kahin_engine_health -args '{}'`

```
tools/call kahin_engine_health: isError=false
{
  "engine": null,
  "error": "No browser engine running.",
  "code": "engine_unavailable",
  "last_engine_death": null
}
```

### 7.4 `go run -flow` — tam tarayıcı akışı (izole kilit + KAHIN_HOME)

```bash
$ rm -rf /tmp/kahin-probe && mkdir -p /tmp/kahin-probe/home
$ go run . -flow -lock /tmp/kahin-probe/browser.lock -home /tmp/kahin-probe/home > run-flow.txt
```

Tam çıktı (`/tmp/kahin-mcp-probe/run-flow.txt`, 121 satır, 3532 bayt,
stderr boş). İki bağımsız koşu yapıldı; protokol aynı, süreler ve rastgele
kimlik hash'i değişken (beklenen — her açılışta yeni BrowserForge parmak izi):

| Adım | Koşu 1 | Koşu 2 |
|---|---|---|
| `kahin_browser_start` | 4.40 s | 4.51 s |
| `kahin_navigate` | 2.81 s | 3.30 s |
| `kahin_engine_health` | 0.00 s | 0.00 s |
| `kahin_mirage_snapshot` | 0.24 s | 0.24 s |
| `kahin_screenshot` | 0.02 s | 0.02 s |
| `kahin_browser_stop` | 0.46 s | 0.46 s |
| `identity.hash` | `0cd2c1a70d2ca64b` | `80dad16cc55fbdf0` |

Aşağıdaki blok Koşu 2'nin dosya içeriğidir (kırpılmamış):

```
initialize OK: server=kahin v0.3.10 protocolVersion=2025-06-18
tools/list OK: 161 tool
  kahin_browser_start          present=true
  kahin_navigate               present=true
  kahin_mirage_snapshot        present=true
  kahin_mirage_click           present=true
  kahin_mirage_fill_form       present=true
  kahin_mirage_dom_action      present=true
  kahin_screenshot             present=true
  kahin_engine_health          present=true
  kahin_mirage_dom_snapshot    present=true

--- kahin_browser_start (4.51s) isError=false ---
{
  "status": "started",
  "engine": "mirage",
  "state_mode": "keş",
  "capabilities": {
    "engine": "mirage",
    "backend": "Camoufox",
    "transport": "juggler-stdio",
    "visual": true, "screenshot": true, "mobile_audit": true,
    "dom_stream": true, "accessibility": true, "screencast": true,
    "auto_promote_to": null
  },
  "identity": {
    "name": null,
    "hash": "80dad16cc55fbdf0",
    "configured": false,
    "stealth": {"headless": true, "humanize": false, "disable_coop": false,
                "enable_cache": true, "block_webgl": false, "main_world_eval": false,
                "os": "linux", "config": {"forceScopeAccess": true},
                "firefox_user_prefs": {"fission.autostart": true,
                                       "fission.webContentIsolationStrategy": 0},
                "proxy": false}
  },
  "profile": {"persistent": false,
              "path": "/tmp/kahin-probe/home/kes/kahin-kes-..."},
  "addons": [],
  "port": 0,
  "tabs": [{"targetId": "20e4cce4-...", "sessionId": "98dbebd3-...", "current": true}],
  "hint": "Reuse this browser; for s...
…[kırpıldı: 900 karakter sınırı — tam metin run-flow.txt'te]

--- kahin_navigate (3.30s) isError=false ---
{
  "frameId": "mainframe-12",
  "loaderId": "nav-25",
  "wait_until": "load",
  "timeout": 30.0
}

--- kahin_engine_health (0.00s) isError=false ---
{
  "engine": "mirage",
  "alive": true,
  "capabilities": { ... "transport": "juggler-stdio" ... },
  "health": {"alive": true, "pid": 3372628, "state": "running"},
  "pid": 3372628,
  "stderr_log": ".../logs/kahin-sidecar-3372540-1790627839524690418.err",
  "currentTarget": "228423f8-675d-45a9-a547-276d4086a235",
  "uptime_s": 4.668,
  "tabCount": 1
}

--- kahin_mirage_snapshot (0.24s) isError=false ---
{
  "lines": [
    "- html \"This domain is for use in documentation examples without nee…\" [ref=nwd0umnfl-1]",
    "  - body \"This domain is for use in documentation examples without nee…\" [ref=nwd0umnfl-3]",
    "    - p \"This domain is for use in documentation examples without nee…\" [ref=nwd0umnfl-10]",
    "      - span \"T\" [ref=nwd0umnfl-11]",
    ...
  ]
}

--- kahin_screenshot (0.02s) isError=false ---
{
  "path": "/home/void0x14/Documents/mcp-projelerim/cdp-kahin-mcp/screenshots/screenshot-1790627846017.png",
  "format": "png",
  "bytes": 22050
}

--- kahin_browser_stop (0.46s) isError=false ---
{"status": "stopped"}
```

**Kanıtlanan:** Go programı Kahin'i başlatıp gerçek Camoufox tarayıcı açtı,
`example.com`'a gitti, DOM snapshot aldı, PNG ekran görüntüsü yazdı ve motoru
kapattı. Süreler gerçek ölçüm.

### 7.5 `kahin_browser_start` zorunlu argümanları (ölçüldü)

`mode` verilmeden:

```json
{"error": "mode zorunlu: 'ağırbaş' (kalıcı) ya da 'keş' (geçici) seç.",
 "code": "mode_required", "tool": "kahin_browser_start", "modes": {...}}
```

`mode:"keş"` ama `ephemeral_ack` yok:

```json
{"error": "keş geçicidir: kahin_browser_stop sonrası hiçbir şey kalmaz.",
 "code": "ephemeral_ack_required", "tool": "kahin_browser_start", "modes": {...}}
```

**Yani:** `kahin_browser_start` için `mode` **zorunlu**; `mode="keş"` seçilirse
`ephemeral_ack=true` da zorunlu. `isError` yine `false` — hata `code` alanında.

### 7.6 Doğrulanamayanlar

- `kahin_mirage_fill_form.fields` içindeki alan adları (`selector`/`value` mi,
  `ref`/`text` mi) — şema `additionalProperties:true` veriyor, canlı deneme
  yapılmadı. **Doğrulanamadı.**
- `kahin_mirage_click`, `kahin_mirage_dom_action`, `kahin_mirage_dom_snapshot`
  canlı çağrısı — `-flow` akışına eklenmedi. **Doğrulanamadı** (şemalar
  `tools/list`'ten alındı, argümanlar speclere uygun).
- `kahin_mirage_dom_action` için canlı `nodeId` — snapshot'taki `[ref=nwd0umnfl-N]`
  biçimi gözlemlendi ama action ile tüketilmedi. **Doğrulanamadı.**

---

## 8. Go tarafı için kontrol listesi

1. `exec.Command(pythonBin, "-m", "kahin.oracle")`, `cmd.Dir` = nötr dizin veya repo kökü.
2. `cmd.Env = append(os.Environ(), extraEnv...)` — ortamı sıfırlamayın.
3. stderr'i ayrı goroutine'de toplayın; stdout yalnız JSON-RPC.
4. `bufio.Reader` buffer'ı ≥1 MB (tools/list büyük).
5. Her istek tek satır JSON + `\n`; **Content-Length yazmayın**.
6. Sıra: `initialize` → `notifications/initialized` → `tools/list` → `tools/call`.
7. `protocolVersion` olarak `"2025-06-18"` (veya `"2025-11-25"`) gönderin.
8. Yanıtları id ile eşleştirin; id'siz bildirimleri atlayın.
9. `structuredContent` **her zaman gelmez**; önce `.result` string'i, sonra `content[0].text`.
10. `isError:false` başarı demek değil — gövdedeki `code`/`error` alanlarını kontrol edin.
11. Makine geneli tarayıcı kilidi: ya mevcut MCP oturumunu durdurun, ya
    `KAHIN_BROWSER_LOCK_PATH` + `KAHIN_HOME` ile ayrı slot açın.
12. `kahin_browser_start` çağrısında `mode` (+ `keş` için `ephemeral_ack:true`) verin.
13. `cmd.Dir` = ayrılmış bir çalışma dizini (ekran görüntüleri oraya düşer);
    `/tmp` gibi yabancı `.so` içeren dizin seçmeyin.
14. `screenshots/` ve `logs/` yollarını **tahmin etmeyin** — tool yanıtındaki
    `path` / `stderr_log` alanlarını okuyun.

---

## 9. Kaynak referansları

| Dosya:satır | İçerik |
|---|---|
| `kahin/oracle.py:117-118` | `main()` gövdesi → `mcp.run(transport="stdio")` |
| `kahin/_mcp.py:28-59` | `FastMCP(name="kahin", instructions=...)` |
| `kahin/_mcp.py:64` | `mcp._mcp_server.version = __version__` |
| `kahin/__init__.py:23` | `__version__ = "0.3.10"` |
| `kahin/_state.py:48-88` | `acquire_browser_lock()` — makine geneli flock |
| `kahin/_state.py:59-64` | `KAHIN_BROWSER_LOCK_PATH` / `XDG_RUNTIME_DIR` çözümü |
| `kahin/tools/engine.py:26,159` | `kahin_engine_health` + `_dump` (orjson) |
| `kahin/tools/pilot.py:437` | `kahin_browser_start` |
| `kahin/tools/pilot.py:1217` | `kahin_navigate` |
| `kahin/tools/pilot.py:1453,1484` | `kahin_screenshot` + `screenshots/` yazımı |
| `kahin/tools/pilot_mirage.py:541` | `kahin_mirage_click` |
| `kahin/tools/agent_mirage.py:301` | `kahin_mirage_fill_form` |
| `kahin/tools/agent_mirage.py:371` | `kahin_mirage_snapshot` |
| `kahin/tools/dom_stream_mirage.py:239` | `kahin_mirage_dom_snapshot` |
| `kahin/tools/dom_stream_mirage.py:370` | `kahin_mirage_dom_action` |
| `kahin/the_twins/mirage.py:125-136` | `_kahin_home()` |
| `kahin/the_twins/mirage.py:934-941` | sidecar stderr `log_dir = parents[2]/logs` |
| `kahin/the_twins/mirage.py:91,161` | `KAHIN_PROFILE_DIR` |
| `pyproject.toml:29` | `kahin = "kahin.oracle:main"` |
| `pyproject.toml:14` | `mcp>=1.0.0,<2` |
| SDK `mcp/server/stdio.py:47,49,63,65,80` | stdio: UTF-8, satır okuma, `\n` yazma |
| SDK `mcp/server/session.py:178-199` | sürüm yansıtma + `InitializationState` |
| SDK `mcp/server/session.py:203-205` | initialize öncesi istek reddi |
| SDK `mcp/types.py:27,35` | `LATEST_PROTOCOL_VERSION`, `DEFAULT_NEGOTIATED_VERSION` |
| SDK `mcp/shared/version.py:3` | `SUPPORTED_PROTOCOL_VERSIONS` |
| SDK `mcp/server/fastmcp/utilities/func_metadata.py:120-132` | `wrap_output` → `{"result": <str>}` |
| SDK `mcp/server/fastmcp/server.py` | `run(transport="stdio")` varsayılanı |

---

## 10. Dosya konumları

| Ne | Nerede |
|---|---|
| Go taslağı | `/tmp/kahin-mcp-probe/main.go` |
| Go modülü | `/tmp/kahin-mcp-probe/go.mod` |
| Derlenmiş binary | `/tmp/kahin-mcp-probe/kahin-mcp-probe` |
| Varsayılan koşu çıktısı | `/tmp/kahin-mcp-probe/run-default.txt` |
| `-call` koşu çıktısı | `/tmp/kahin-mcp-probe/run-call.txt` |
| Kurulu venv koşu çıktısı | `/tmp/kahin-mcp-probe/run-installed-venv.txt` |
| Tam akış çıktısı (121 satır) | `/tmp/kahin-mcp-probe/run-flow.txt` |
| 9 tool'un ham şeması | `/tmp/kahin-mcp-probe/schemas.json` |

**Not:** `/tmp` kalıcı değildir. Kalıcı kullanım için `main.go` ve `go.mod`
proje ağacına taşınmalı.

---

## 11. Tek cümlelik operasyonel özet

Go'dan `python -m kahin.oracle`'ı `os/exec` ile başlat, stdin'e satır başına
JSON-RPC yaz (`initialize` → `initialized` → `tools/list` → `tools/call`),
stdout'tan satır başına JSON-RPC oku; tool sonucu `content[0].text` veya
`structuredContent.result` içinde **JSON metni** olarak gelir. Makine geneli
tarayıcı kilidi nedeniyle aynı anda tek MCP süreci tarayıcı açabilir; paralel
kullanım için `KAHIN_BROWSER_LOCK_PATH` + `KAHIN_HOME` ile ayrı slot ver.

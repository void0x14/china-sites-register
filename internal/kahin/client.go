// Package kahin, Kahin MCP sunucusunu (stdio transport) Go'dan sürer.
//
// Protokol, /tmp/kahin-mcp-probe/main.go ile canlı olarak doğrulandı ve
// docs/kahin-mcp-protokol.md içinde kaynak satır referanslarıyla belgelendi:
//
//   - Transport: stdio. Tek çerçeveleme: newline-delimited JSON.
//     kahin/oracle.py:118  ->  mcp.run(transport="stdio")
//     mcp/server/stdio.py:63,80 -> satır satır oku, json + "\n" yaz
//     Content-Length (LSP tarzı) ÇALIŞMAZ; negatif testle doğrulandı.
//
//   - Tool sonucu: content[0].text ve structuredContent.result AYNI JSON
//     METNİDİR. structuredContent düz obje değil {"result": "<string>"}
//     zarfıdır; önce .result string'i alınıp sonra JSON parse edilir.
//     isError=false "iş başarılı" demek DEĞİLDİR; gövdedeki code/error
//     ayrıca kontrol edilir.
//
//   - initialize zorunlu; notifications/initialized toleranslı ama gönderilir.
//
//   - Makine geneli TEK tarayıcı slotu vardır (kahin/_state.py:48 flock).
//     Paralel/bağımsız kullanım için KAHIN_BROWSER_LOCK_PATH ve KAHIN_HOME
//     ayrı verilmelidir.
package kahin

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// JSON-RPC 2.0
// ---------------------------------------------------------------------------

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ToolResult, tools/call yanıtıdır.
type ToolResult struct {
	Content           []contentBlock `json:"content"`
	StructuredContent map[string]any `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError"`
}

// Text, tool çıktısını JSON metni olarak döndürür.
//
// KANIT (canlı çağrı): Kahin tool'ları Python'da `-> str` döner; FastMCP
// (mcp/server/fastmcp/utilities/func_metadata.py:120-132) bunu hem
// content[0].text hem structuredContent.result içine AYNI string olarak koyar.
func (r *ToolResult) Text() string {
	if r == nil {
		return ""
	}
	if v, ok := r.StructuredContent["result"].(string); ok && v != "" {
		return v
	}
	for _, b := range r.Content {
		if b.Type == "text" && b.Text != "" {
			return b.Text
		}
	}
	return ""
}

// Decode, tool çıktısını (JSON metni) hedef yapıya çözer.
func (r *ToolResult) Decode(v any) error {
	t := r.Text()
	if t == "" {
		return fmt.Errorf("kahin: boş tool çıktısı")
	}
	if err := json.Unmarshal([]byte(t), v); err != nil {
		return fmt.Errorf("kahin: tool çıktısı JSON değil: %w (ilk 200: %.200s)", err, t)
	}
	return nil
}

// Envelope, tool çıktılarında ortak hata zarfını yakalar.
// Kahin hataları isError=false ile de dönebilir; code/error ayrı okunur.
type Envelope struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr io.ReadCloser

	mu     sync.Mutex
	nextID int64

	stderrMu  sync.Mutex
	stderrLog []string

	closed bool
}

// Options, alt süreci başlatma parametreleridir.
type Options struct {
	// Python, Kahin paketini import edebilen python yorumlayıcısı.
	Python string
	// Dir, alt sürecin çalışma dizini (kahin paketi import edilebilir olmalı).
	Dir string
	// Env, devralınan ortama eklenen değişkenler (KAHIN_HOME, KAHIN_BROWSER_LOCK_PATH...).
	Env []string
	// ClientName / ClientVersion, initialize clientInfo.
	ClientName    string
	ClientVersion string
}

// NewClient, Kahin MCP sunucusunu stdio transport ile başlatır.
func NewClient(o Options) (*Client, error) {
	if o.Python == "" {
		return nil, fmt.Errorf("kahin: python yolu boş")
	}
	if o.Dir == "" {
		return nil, fmt.Errorf("kahin: çalışma dizini boş")
	}
	if o.ClientName == "" {
		o.ClientName = "china-sites-register"
	}
	if o.ClientVersion == "" {
		o.ClientVersion = "0.1.0"
	}

	cmd := exec.Command(o.Python, "-m", "kahin.oracle")
	cmd.Dir = o.Dir
	// KRİTİK: ortamı sıfırdan kurma; HOME/XDG_*/DISPLAY devralınmalı.
	cmd.Env = append(os.Environ(), o.Env...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("kahin: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("kahin: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("kahin: stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("kahin: başlatma: %w", err)
	}

	c := &Client{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReaderSize(stdout, 1<<20), // tools/list yanıtı 200KB+
		stderr: stderr,
	}

	// stderr ayrı toplanır; Kahin tanılamayı buraya yazar, protokole karışmaz.
	go func() {
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 1<<16), 1<<22)
		for sc.Scan() {
			c.stderrMu.Lock()
			c.stderrLog = append(c.stderrLog, sc.Text())
			if len(c.stderrLog) > 500 {
				c.stderrLog = c.stderrLog[len(c.stderrLog)-500:]
			}
			c.stderrMu.Unlock()
		}
	}()

	if err := c.initialize(o.ClientName, o.ClientVersion); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// StderrLog, toplanan son stderr satırlarını döndürür.
func (c *Client) StderrLog() []string {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	out := make([]string, len(c.stderrLog))
	copy(out, c.stderrLog)
	return out
}

func (c *Client) call(method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, fmt.Errorf("kahin: istemci kapalı")
	}

	c.nextID++
	id := c.nextID
	req := rpcRequest{JSONRPC: "2.0", ID: &id, Method: method, Params: params}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("kahin: %s encode: %w", method, err)
	}
	if _, err := c.stdin.Write(append(raw, '\n')); err != nil {
		return nil, fmt.Errorf("kahin: %s yazma: %w", method, err)
	}

	for {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("kahin: %s okuma: %w (%s)", method, err, c.lastStderr())
		}
		if len(line) == 0 {
			continue
		}
		var resp rpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			// Protokol dışı stdout satırı: atla ama görünür kıl.
			fmt.Fprintf(os.Stderr, "[kahin] JSON olmayan stdout satırı: %.200s\n", line)
			continue
		}
		if resp.ID == nil {
			continue // sunucu bildirimi
		}
		if *resp.ID != id {
			continue // başka isteğe ait
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("kahin: %s rpc hata %d: %s", method, resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

func (c *Client) notify(method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("kahin: istemci kapalı")
	}
	req := rpcRequest{JSONRPC: "2.0", Method: method, Params: params}
	raw, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = c.stdin.Write(append(raw, '\n'))
	return err
}

func (c *Client) lastStderr() string {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	n := len(c.stderrLog)
	if n == 0 {
		return "stderr boş"
	}
	if n > 5 {
		n = 5
	}
	return strings.Join(c.stderrLog[len(c.stderrLog)-n:], " | ")
}

func (c *Client) initialize(name, version string) error {
	params := map[string]any{
		// Sunucu istenen sürümü destekliyorsa aynen yansıtır; desteklemiyorsa
		// 2025-11-25 döner (mcp/server/session.py:178-187).
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": name, "version": version},
	}
	if _, err := c.call("initialize", params); err != nil {
		return fmt.Errorf("kahin: initialize: %w", err)
	}
	return c.notify("notifications/initialized", nil)
}

// ToolInfo, tools/list kaydıdır.
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ListTools, sunucudaki tool'ları döndürür.
func (c *Client) ListTools() ([]ToolInfo, error) {
	raw, err := c.call("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var res struct {
		Tools []ToolInfo `json:"tools"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return res.Tools, nil
}

// Call, bir tool çağırır.
func (c *Client) Call(name string, args map[string]any) (*ToolResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	raw, err := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return nil, err
	}
	var res ToolResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// Close, alt süreci kapatır.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	_ = c.stdin.Close()
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		_ = c.cmd.Process.Kill()
		return nil
	}
}

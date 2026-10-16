package ws

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// RFC 6455 WebSocket 魔数 GUID。
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// WebSocket 帧操作码（Opcode）。
const (
	OpcodeContinuation = 0x0
	OpcodeText         = 0x1
	OpcodeBinary       = 0x2
	OpcodeClose        = 0x8
	OpcodePing         = 0x9
	OpcodePong         = 0xA
)

// 默认最大单条消息大小（32MB），防止恶意超大帧耗尽内存。
const defaultMaxMessageSize = 32 << 20

// HandshakeError 表示 WebSocket 握手阶段的 HTTP 非 101 响应错误。
//
// 当服务端拒绝协议升级（如返回 400 Bad Request、401 Unauthorized、404 Not Found 等）时，
// [Dial] 会返回此错误类型。调用方可通过类型断言或 errors.As 提取服务端返回的 HTTP 状态码与响应体。
type HandshakeError struct {
	// StatusCode 是服务端返回的 HTTP 响应状态码（非 101）。
	StatusCode int

	// Body 是服务端返回的完整原始响应体字节切片。
	Body []byte
}

func (e *HandshakeError) Error() string {
	if len(e.Body) > 0 {
		return fmt.Sprintf("websocket handshake failed with status %d: %s", e.StatusCode, string(e.Body))
	}
	return fmt.Sprintf("websocket handshake failed with status %d", e.StatusCode)
}

// Conn 封装一条符合 RFC 6455 的轻量级 WebSocket 连接（纯标准库实现，零外部依赖）。
//
// 支持客户端模式（isClient=true，发送帧自动加掩码）与服务端模式（isClient=false，发送帧不加掩码）。
// WriteMessage 内部加互斥锁，可安全并发调用；ReadMessage 由单个读取循环调用。
type Conn struct {
	netConn  net.Conn
	br       *bufio.Reader
	writeMu  sync.Mutex
	isClient bool
	closed   bool
}

// Dial 发起客户端 WebSocket 握手并返回已建立双向连接的 [*Conn]。
//
// 参数：
//   - ctx: 上下文，控制 TCP 建连、TLS 握手以及 HTTP 升级请求的整体超时与取消；
//   - rawURL: 目标服务端地址（支持 ws:// 与 wss://；传入 http:// 或 https:// 会被自动纠正为对应 WS 协议）；
//   - headers: 附加 HTTP 请求头（如 Authorization、User-Agent 等；Sec-WebSocket-* 头由内部自动生成）。
//
// 错误处理：
//   - 若目标服务端返回非 101 状态码，返回包含状态码与响应体的 [*HandshakeError]；
//   - 若网络不可达、TLS 证书不合法或握手密钥回执不匹配，返回对应底层的 error。
func Dial(ctx context.Context, rawURL string, headers map[string]string) (*Conn, error) {
	u, err := normalizeWSURL(rawURL)
	if err != nil {
		return nil, err
	}

	hostPort := u.Host
	if u.Port() == "" {
		if u.Scheme == "wss" {
			hostPort = net.JoinHostPort(u.Hostname(), "443")
		} else {
			hostPort = net.JoinHostPort(u.Hostname(), "80")
		}
	}

	var d net.Dialer
	netConn, err := d.DialContext(ctx, "tcp", hostPort)
	if err != nil {
		return nil, err
	}

	if u.Scheme == "wss" {
		tlsConn := tls.Client(netConn, &tls.Config{
			ServerName: u.Hostname(),
			MinVersion: tls.VersionTLS12,
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = netConn.Close()
			return nil, err
		}
		netConn = tlsConn
	}

	// 构造 Sec-WebSocket-Key
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		_ = netConn.Close()
		return nil, err
	}
	secKey := base64.StdEncoding.EncodeToString(keyBytes)

	reqURI := u.RequestURI()
	if reqURI == "" {
		reqURI = "/"
	}

	var reqBuf bytes.Buffer
	fmt.Fprintf(&reqBuf, "GET %s HTTP/1.1\r\n", reqURI)
	fmt.Fprintf(&reqBuf, "Host: %s\r\n", u.Host)
	reqBuf.WriteString("Upgrade: websocket\r\n")
	reqBuf.WriteString("Connection: Upgrade\r\n")
	fmt.Fprintf(&reqBuf, "Sec-WebSocket-Key: %s\r\n", secKey)
	reqBuf.WriteString("Sec-WebSocket-Version: 13\r\n")
	for k, v := range headers {
		if strings.EqualFold(k, "Host") ||
			strings.EqualFold(k, "Upgrade") ||
			strings.EqualFold(k, "Connection") ||
			strings.EqualFold(k, "Sec-WebSocket-Key") ||
			strings.EqualFold(k, "Sec-WebSocket-Version") {
			continue
		}
		fmt.Fprintf(&reqBuf, "%s: %s\r\n", k, v)
	}
	reqBuf.WriteString("\r\n")

	if deadline, ok := ctx.Deadline(); ok {
		_ = netConn.SetDeadline(deadline)
	}
	if _, err := netConn.Write(reqBuf.Bytes()); err != nil {
		_ = netConn.Close()
		return nil, err
	}

	br := bufio.NewReaderSize(netConn, 32*1024)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil {
		_ = netConn.Close()
		return nil, err
	}
	_ = netConn.SetDeadline(time.Time{})

	if resp.StatusCode != http.StatusSwitchingProtocols {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		_ = resp.Body.Close()
		_ = netConn.Close()
		return nil, &HandshakeError{
			StatusCode: resp.StatusCode,
			Body:       body,
		}
	}

	wantAccept := computeAcceptKey(secKey)
	if gotAccept := strings.TrimSpace(resp.Header.Get("Sec-WebSocket-Accept")); gotAccept != wantAccept {
		_ = netConn.Close()
		return nil, fmt.Errorf("websocket invalid Sec-WebSocket-Accept: got %q, want %q", gotAccept, wantAccept)
	}

	return &Conn{
		netConn:  netConn,
		br:       br,
		isClient: true,
	}, nil
}

// Upgrade 将服务端收到的标准 HTTP 请求升级为 WebSocket 连接（RFC 6455）。
//
// 内部执行步骤：
//  1. 校验请求头 Connection 是否包含 Upgrade，以及 Upgrade 是否为 websocket；
//  2. 提取并校验客户端 Sec-WebSocket-Key 是否合法存在；
//  3. 通过 [http.Hijacker] 劫持底层 TCP 连接；
//  4. 按照 RFC 6455 规范计算 Sec-WebSocket-Accept 并向客户端写入 HTTP 101 Switching Protocols 响应头。
//
// 参数：
//   - w: 当前 HTTP 请求的 ResponseWriter（必须实现 [http.Hijacker] 接口）；
//   - r: 当前接收到的 HTTP 请求指针。
//
// 成功时返回配置为服务端模式（isClient=false）的 [*Conn]；失败时向客户端写入相应 HTTP 错误并返回 error。
func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if !headerContainsToken(r.Header, "Connection", "upgrade") ||
		!headerContainsToken(r.Header, "Upgrade", "websocket") {
		http.Error(w, "websocket upgrade required", http.StatusUpgradeRequired)
		return nil, errors.New("websocket: missing Upgrade or Connection header")
	}
	secKey := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if secKey == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, errors.New("websocket: missing Sec-WebSocket-Key")
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "webserver does not support hijacking", http.StatusInternalServerError)
		return nil, errors.New("websocket: response writer does not implement http.Hijacker")
	}

	netConn, rw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}

	accept := computeAcceptKey(secKey)
	var respBuf bytes.Buffer
	respBuf.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	respBuf.WriteString("Upgrade: websocket\r\n")
	respBuf.WriteString("Connection: Upgrade\r\n")
	fmt.Fprintf(&respBuf, "Sec-WebSocket-Accept: %s\r\n\r\n", accept)

	if _, err := netConn.Write(respBuf.Bytes()); err != nil {
		_ = netConn.Close()
		return nil, err
	}

	return &Conn{
		netConn:  netConn,
		br:       rw.Reader,
		isClient: false,
	}, nil
}

// ReadMessage 读取下一条完整的文本或二进制消息。
//
// 核心行为：
//   - 分片续接：自动组装连续的 Continuation 分片帧，直到 FIN 标志为 true 的完整消息；
//   - 自动保活：收到 OpcodePing 时在内部自动应答 OpcodePong 帧；收到 OpcodePong 时静默丢弃；
//   - 关闭处理：收到 OpcodeClose 时自动回发关闭帧、关闭底层网络连接并返回 [io.EOF]；
//   - 长度保护：单条消息超过默认上限 32MB 时返回错误，防止恶意攻击。
//
// 并发约束：
//
//	ReadMessage 非并发安全，同一时间必须由单个 goroutine 顺序读取。
func (c *Conn) ReadMessage() (opcode byte, payload []byte, err error) {
	var msgBuf bytes.Buffer
	var msgOpcode byte
	inFragment := false

	for {
		fin, op, frameData, frameErr := c.readFrame()
		if frameErr != nil {
			return 0, nil, frameErr
		}

		switch op {
		case OpcodePing:
			_ = c.writeFrame(OpcodePong, frameData)
			continue
		case OpcodePong:
			continue
		case OpcodeClose:
			_ = c.writeFrame(OpcodeClose, nil)
			_ = c.Close()
			return OpcodeClose, nil, io.EOF
		case OpcodeText, OpcodeBinary:
			if inFragment {
				return 0, nil, errors.New("websocket: unexpected data opcode inside fragmented message")
			}
			msgOpcode = op
			msgBuf.Write(frameData)
			if fin {
				return msgOpcode, msgBuf.Bytes(), nil
			}
			inFragment = true
		case OpcodeContinuation:
			if !inFragment {
				return 0, nil, errors.New("websocket: unexpected continuation frame")
			}
			if msgBuf.Len()+len(frameData) > defaultMaxMessageSize {
				return 0, nil, errors.New("websocket: fragmented message exceeds max size")
			}
			msgBuf.Write(frameData)
			if fin {
				return msgOpcode, msgBuf.Bytes(), nil
			}
		default:
			return 0, nil, fmt.Errorf("websocket: unsupported opcode %d", op)
		}
	}
}

// WriteText 发送一条 UTF-8 文本消息（OpcodeText）。
//
// 客户端模式下发送会自动生成随机 4 字节掩码；服务端模式下发送不带掩码。
// 内部使用互斥锁保护，可由多个 goroutine 并发安全调用。
func (c *Conn) WriteText(data []byte) error {
	return c.writeFrame(OpcodeText, data)
}

// Close 优雅关闭 WebSocket 连接。
//
// 首先并发安全地尝试发送一次标准关闭帧（OpcodeClose），随后关闭底层 TCP 网络连接。
// 多次重复调用是安全的且不会发生 panic。
func (c *Conn) Close() error {
	c.writeMu.Lock()
	if !c.closed {
		c.closed = true
		_ = c.writeFrameLocked(OpcodeClose, nil)
	}
	c.writeMu.Unlock()
	return c.netConn.Close()
}

func (c *Conn) readFrame() (fin bool, opcode byte, payload []byte, err error) {
	var hdr [2]byte
	if _, err := io.ReadFull(c.br, hdr[:]); err != nil {
		return false, 0, nil, err
	}
	fin = (hdr[0] & 0x80) != 0
	opcode = hdr[0] & 0x0F
	masked := (hdr[1] & 0x80) != 0
	length := uint64(hdr[1] & 0x7F)

	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return false, 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return false, 0, nil, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}

	if length > defaultMaxMessageSize {
		return false, 0, nil, fmt.Errorf("websocket: frame payload too large (%d bytes)", length)
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(c.br, maskKey[:]); err != nil {
			return false, 0, nil, err
		}
	}

	payload = make([]byte, int(length))
	if length > 0 {
		if _, err := io.ReadFull(c.br, payload); err != nil {
			return false, 0, nil, err
		}
		if masked {
			for i := range payload {
				payload[i] ^= maskKey[i%4]
			}
		}
	}

	return fin, opcode, payload, nil
}

func (c *Conn) writeFrame(opcode byte, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.closed {
		return net.ErrClosed
	}
	return c.writeFrameLocked(opcode, payload)
}

func (c *Conn) writeFrameLocked(opcode byte, payload []byte) error {
	var buf bytes.Buffer
	b0 := byte(0x80) | (opcode & 0x0F) // FIN = 1
	buf.WriteByte(b0)

	maskBit := byte(0)
	if c.isClient {
		maskBit = 0x80
	}

	n := len(payload)
	switch {
	case n <= 125:
		buf.WriteByte(maskBit | byte(n))
	case n <= 65535:
		buf.WriteByte(maskBit | 126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(n))
		buf.Write(ext[:])
	default:
		buf.WriteByte(maskBit | 127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		buf.Write(ext[:])
	}

	if c.isClient {
		var maskKey [4]byte
		_, _ = rand.Read(maskKey[:])
		buf.Write(maskKey[:])
		masked := make([]byte, n)
		for i := 0; i < n; i++ {
			masked[i] = payload[i] ^ maskKey[i%4]
		}
		buf.Write(masked)
	} else if n > 0 {
		buf.Write(payload)
	}

	_, err := c.netConn.Write(buf.Bytes())
	return err
}

func computeAcceptKey(secKey string) string {
	h := sha1.New()
	h.Write([]byte(secKey))
	h.Write([]byte(wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func normalizeWSURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		u.Scheme = strings.ToLower(u.Scheme)
	default:
		return nil, fmt.Errorf("websocket: unsupported URL scheme %q", u.Scheme)
	}
	return u, nil
}

func headerContainsToken(h http.Header, key, token string) bool {
	for _, v := range h.Values(key) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

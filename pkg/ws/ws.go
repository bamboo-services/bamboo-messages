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
type HandshakeError struct {
	StatusCode int
	Body       []byte
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

// Dial 发起客户端 WebSocket 握手并返回连接（支持 ws:// 与 wss://，自动将 http/https 转换为 ws/wss）。
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

// Upgrade 将服务端收到的 HTTP 请求升级为 WebSocket 连接（RFC 6455）。
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
// 自动处理分片续接（Continuation frames）、自动回复 Ping 帧（返回 Pong）以及处理 Close 帧。
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

// WriteText 发送一条 UTF-8 文本消息（JSON 事件帧）。
func (c *Conn) WriteText(data []byte) error {
	return c.writeFrame(OpcodeText, data)
}

// Close 发送关闭帧并关闭底层网络连接。
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

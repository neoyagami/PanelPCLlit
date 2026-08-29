package obsws

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type Client struct {
	mu       sync.Mutex
	url      string
	password string
	conn     net.Conn
	reader   *bufio.Reader
	nextID   uint64
	lastErr  string
}

type envelope struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
}

type helloData struct {
	RPCVersion     int `json:"rpcVersion"`
	Authentication *struct {
		Challenge string `json:"challenge"`
		Salt      string `json:"salt"`
	} `json:"authentication"`
}

type responseData struct {
	RequestType   string          `json:"requestType"`
	RequestID     string          `json:"requestId"`
	ResponseData  json.RawMessage `json:"responseData"`
	RequestStatus struct {
		Result  bool   `json:"result"`
		Code    int    `json:"code"`
		Comment string `json:"comment"`
	} `json:"requestStatus"`
}

func New(rawURL, password string) *Client {
	return &Client{url: rawURL, password: password}
}

func (c *Client) Configure(rawURL, password string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if rawURL != c.url || password != c.password {
		c.closeLocked()
	}
	c.url, c.password = rawURL, password
}

func (c *Client) LastError() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastErr
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
}

func (c *Client) Test() error {
	return c.Call("GetVersion", map[string]any{})
}

func (c *Client) Call(requestType string, requestData map[string]any) error {
	_, err := c.Request(requestType, requestData)
	return err
}

func (c *Client) Request(requestType string, requestData map[string]any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := c.callLocked(requestType, requestData)
	if err != nil {
		c.closeLocked()
		c.lastErr = err.Error()
	} else {
		c.lastErr = ""
	}
	return data, err
}

func (c *Client) callLocked(requestType string, requestData map[string]any) (json.RawMessage, error) {
	if c.conn == nil {
		if err := c.connectLocked(); err != nil {
			return nil, err
		}
	}
	if err := c.conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return nil, err
	}
	defer c.conn.SetDeadline(time.Time{})
	c.nextID++
	id := strconv.FormatUint(c.nextID, 10)
	request := map[string]any{
		"op": 6,
		"d": map[string]any{
			"requestType": requestType,
			"requestId":   id,
			"requestData": requestData,
		},
	}
	if err := c.writeJSONLocked(request); err != nil {
		return nil, fmt.Errorf("enviar solicitud OBS: %w", err)
	}
	for {
		env, err := c.readEnvelopeLocked()
		if err != nil {
			return nil, fmt.Errorf("leer respuesta OBS: %w", err)
		}
		if env.Op != 7 {
			continue
		}
		var data responseData
		if err := json.Unmarshal(env.D, &data); err != nil {
			return nil, err
		}
		if data.RequestID != id {
			continue
		}
		if !data.RequestStatus.Result {
			return nil, fmt.Errorf("OBS %s (%d): %s", requestType, data.RequestStatus.Code, data.RequestStatus.Comment)
		}
		return data.ResponseData, nil
	}
}

func (c *Client) connectLocked() error {
	rawURL := c.url
	if rawURL == "" {
		rawURL = "ws://127.0.0.1:4455"
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL de OBS inválida: %w", err)
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return errors.New("la URL de OBS debe comenzar con ws:// o wss://")
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		if u.Scheme == "wss" {
			host += ":443"
		} else {
			host += ":80"
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return fmt.Errorf("conectar a %s: %w", host, err)
	}
	if u.Scheme == "wss" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return fmt.Errorf("TLS de OBS: %w", err)
		}
		conn = tlsConn
	}
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	reader, err := websocketHandshake(conn, u)
	if err != nil {
		conn.Close()
		return err
	}
	c.conn, c.reader = conn, reader

	hello, err := c.readEnvelopeLocked()
	if err != nil {
		c.closeLocked()
		return fmt.Errorf("saludo de OBS: %w", err)
	}
	if hello.Op != 0 {
		c.closeLocked()
		return fmt.Errorf("OBS envió op %d en vez de Hello", hello.Op)
	}
	var data helloData
	if err := json.Unmarshal(hello.D, &data); err != nil {
		c.closeLocked()
		return err
	}
	identify := map[string]any{"rpcVersion": 1, "eventSubscriptions": 0}
	if data.Authentication != nil {
		identify["authentication"] = authentication(c.password, data.Authentication.Salt, data.Authentication.Challenge)
	}
	if err := c.writeJSONLocked(map[string]any{"op": 1, "d": identify}); err != nil {
		c.closeLocked()
		return err
	}
	identified, err := c.readEnvelopeLocked()
	if err != nil {
		c.closeLocked()
		return fmt.Errorf("autenticación de OBS: %w", err)
	}
	if identified.Op != 2 {
		c.closeLocked()
		return fmt.Errorf("OBS rechazó la identificación (op %d)", identified.Op)
	}
	c.conn.SetDeadline(time.Time{})
	return nil
}

func authentication(password, salt, challenge string) string {
	secretHash := sha256.Sum256([]byte(password + salt))
	secret := base64.StdEncoding.EncodeToString(secretHash[:])
	authHash := sha256.Sum256([]byte(secret + challenge))
	return base64.StdEncoding.EncodeToString(authHash[:])
}

func websocketHandshake(conn net.Conn, u *url.URL) (*bufio.Reader, error) {
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	req, err := http.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	req.Host = u.Host
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", key)
	req.Header.Set("Sec-WebSocket-Version", "13")
	if err := req.Write(conn); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("OBS respondió HTTP %s", resp.Status)
	}
	wantRaw := sha1.Sum([]byte(key + websocketGUID))
	want := base64.StdEncoding.EncodeToString(wantRaw[:])
	if !strings.EqualFold(resp.Header.Get("Sec-WebSocket-Accept"), want) {
		return nil, errors.New("handshake WebSocket de OBS inválido")
	}
	return reader, nil
}

func (c *Client) writeJSONLocked(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeFrame(c.conn, 1, data)
}

func (c *Client) readEnvelopeLocked() (envelope, error) {
	for {
		opcode, data, err := readFrame(c.reader)
		if err != nil {
			return envelope{}, err
		}
		switch opcode {
		case 1:
			var env envelope
			if err := json.Unmarshal(data, &env); err != nil {
				return envelope{}, err
			}
			return env, nil
		case 8:
			if len(data) >= 2 {
				code := binary.BigEndian.Uint16(data[:2])
				reason := strings.TrimSpace(string(data[2:]))
				if reason != "" {
					return envelope{}, fmt.Errorf("WebSocket cerrado por OBS (%d): %s", code, reason)
				}
				return envelope{}, fmt.Errorf("WebSocket cerrado por OBS (%d)", code)
			}
			return envelope{}, io.EOF
		case 9:
			if err := writeFrame(c.conn, 10, data); err != nil {
				return envelope{}, err
			}
		}
	}
}

func writeFrame(w io.Writer, opcode byte, payload []byte) error {
	var header bytes.Buffer
	header.WriteByte(0x80 | opcode)
	length := len(payload)
	switch {
	case length < 126:
		header.WriteByte(0x80 | byte(length))
	case length <= 0xffff:
		header.WriteByte(0x80 | 126)
		binary.Write(&header, binary.BigEndian, uint16(length))
	default:
		header.WriteByte(0x80 | 127)
		binary.Write(&header, binary.BigEndian, uint64(length))
	}
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}
	header.Write(mask)
	masked := make([]byte, len(payload))
	for i := range payload {
		masked[i] = payload[i] ^ mask[i%4]
	}
	if _, err := w.Write(header.Bytes()); err != nil {
		return err
	}
	_, err := w.Write(masked)
	return err
}

func readFrame(r *bufio.Reader) (byte, []byte, error) {
	first, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	second, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	if first&0x80 == 0 {
		return 0, nil, errors.New("frames WebSocket fragmentados no soportados")
	}
	length := uint64(second & 0x7f)
	if length == 126 {
		var n uint16
		if err := binary.Read(r, binary.BigEndian, &n); err != nil {
			return 0, nil, err
		}
		length = uint64(n)
	} else if length == 127 {
		if err := binary.Read(r, binary.BigEndian, &length); err != nil {
			return 0, nil, err
		}
	}
	if length > 1024*1024 {
		return 0, nil, errors.New("frame WebSocket demasiado grande")
	}
	var mask [4]byte
	masked := second&0x80 != 0
	if masked {
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return 0, nil, err
		}
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return first & 0x0f, payload, nil
}

func (c *Client) closeLocked() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn, c.reader = nil, nil
}

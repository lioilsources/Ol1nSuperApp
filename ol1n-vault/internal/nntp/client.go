package nntp

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config describes how to dial the UseNet provider.
type Config struct {
	Host        string
	Port        int
	TLS         bool
	User        string
	Pass        string
	Connections int // pool size
	Newsgroup   string
	DialTimeout time.Duration
}

// Client is a pooled NNTP client for a single provider.
type Client struct {
	cfg Config
	mu  sync.Mutex
	// idle is a bounded buffered channel acting as a semaphore + pool.
	idle chan *conn
	// sema limits total concurrent connections.
	sema chan struct{}
}

func NewClient(cfg Config) *Client {
	if cfg.Connections <= 0 {
		cfg.Connections = 4
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 30 * time.Second
	}
	return &Client{
		cfg:  cfg,
		idle: make(chan *conn, cfg.Connections),
		sema: make(chan struct{}, cfg.Connections),
	}
}

func (c *Client) Close() error {
	close(c.sema)
	for {
		select {
		case cn := <-c.idle:
			cn.close()
		default:
			return nil
		}
	}
}

// Newsgroup returns the configured upload target group.
func (c *Client) Newsgroup() string { return c.cfg.Newsgroup }

// acquire blocks until a connection is available.
func (c *Client) acquire(ctx context.Context) (*conn, error) {
	select {
	case cn := <-c.idle:
		if cn.isAlive() {
			return cn, nil
		}
		cn.close()
		<-c.sema
	default:
	}
	select {
	case c.sema <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	cn, err := dial(ctx, c.cfg)
	if err != nil {
		<-c.sema
		return nil, err
	}
	return cn, nil
}

func (c *Client) release(cn *conn, keep bool) {
	if cn == nil {
		return
	}
	if !keep || !cn.isAlive() {
		cn.close()
		<-c.sema
		return
	}
	select {
	case c.idle <- cn:
	default:
		cn.close()
		<-c.sema
	}
}

// PostArticle publishes an article body (already yEnc-encoded) to the
// configured newsgroup. Returns the generated Message-ID without angle brackets.
func (c *Client) PostArticle(ctx context.Context, subject string, body []byte) (string, error) {
	cn, err := c.acquire(ctx)
	if err != nil {
		return "", err
	}
	keep := false
	defer func() { c.release(cn, keep) }()

	msgID, err := cn.post(ctx, c.cfg.Newsgroup, subject, c.cfg.User, body)
	if err != nil {
		return "", err
	}
	keep = true
	return msgID, nil
}

// FetchBody retrieves the body of a message by Message-ID.
// Returned bytes are the article body with CR/LF line endings preserved and
// dot-stuffing removed.
func (c *Client) FetchBody(ctx context.Context, messageID string) ([]byte, error) {
	cn, err := c.acquire(ctx)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() { c.release(cn, keep) }()

	body, err := cn.body(ctx, messageID)
	if err != nil {
		return nil, err
	}
	keep = true
	return body, nil
}

// --- connection ---

type conn struct {
	nc   net.Conn
	tp   *textproto.Conn
	bw   *bufio.Writer
	br   *bufio.Reader
	dead bool
}

func dial(ctx context.Context, cfg Config) (*conn, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	d := net.Dialer{Timeout: cfg.DialTimeout}
	var nc net.Conn
	var err error
	if cfg.TLS {
		nc, err = tls.DialWithDialer(&d, "tcp", addr, &tls.Config{ServerName: cfg.Host})
	} else {
		nc, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("nntp: dial %s: %w", addr, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = nc.SetDeadline(deadline)
	}
	tp := textproto.NewConn(nc)
	// Server greeting.
	code, _, err := tp.ReadCodeLine(-1)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nntp: greeting: %w", err)
	}
	if code != 200 && code != 201 {
		nc.Close()
		return nil, fmt.Errorf("nntp: greeting code %d", code)
	}

	c := &conn{nc: nc, tp: tp}
	if cfg.User != "" {
		if err := c.authenticate(cfg.User, cfg.Pass); err != nil {
			nc.Close()
			return nil, err
		}
	}
	// Reset deadline — per-op deadlines take over from here.
	_ = nc.SetDeadline(time.Time{})
	return c, nil
}

func (c *conn) authenticate(user, pass string) error {
	if _, _, err := c.cmd("AUTHINFO USER " + user); err != nil {
		return fmt.Errorf("nntp: AUTHINFO USER: %w", err)
	}
	code, _, err := c.cmd("AUTHINFO PASS " + pass)
	if err != nil {
		return fmt.Errorf("nntp: AUTHINFO PASS: %w", err)
	}
	if code != 281 {
		return fmt.Errorf("nntp: auth rejected: %d", code)
	}
	return nil
}

func (c *conn) cmd(line string) (int, string, error) {
	id, err := c.tp.Cmd("%s", line)
	if err != nil {
		c.dead = true
		return 0, "", err
	}
	c.tp.StartResponse(id)
	defer c.tp.EndResponse(id)
	code, msg, err := c.tp.ReadCodeLine(-1)
	if err != nil {
		c.dead = true
	}
	return code, msg, err
}

func (c *conn) post(ctx context.Context, group, subject, from string, body []byte) (string, error) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.nc.SetDeadline(deadline)
		defer c.nc.SetDeadline(time.Time{})
	}

	if group != "" {
		if _, _, err := c.cmd("GROUP " + group); err != nil {
			return "", err
		}
	}

	code, _, err := c.cmd("POST")
	if err != nil {
		return "", err
	}
	if code != 340 {
		return "", fmt.Errorf("nntp: POST refused: %d", code)
	}

	msgID := generateMessageID()
	writer := c.tp.DotWriter()
	hdr := fmt.Sprintf(
		"From: %s\r\nNewsgroups: %s\r\nSubject: %s\r\nMessage-ID: <%s>\r\nDate: %s\r\n\r\n",
		from, group, subject, msgID, time.Now().UTC().Format(time.RFC1123Z))
	if _, err := writer.Write([]byte(hdr)); err != nil {
		c.dead = true
		return "", err
	}
	if _, err := writer.Write(body); err != nil {
		c.dead = true
		return "", err
	}
	if err := writer.Close(); err != nil {
		c.dead = true
		return "", err
	}
	code, _, err = c.tp.ReadCodeLine(-1)
	if err != nil {
		c.dead = true
		return "", err
	}
	if code != 240 {
		return "", fmt.Errorf("nntp: POST rejected: %d", code)
	}
	return msgID, nil
}

func (c *conn) body(ctx context.Context, messageID string) ([]byte, error) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.nc.SetDeadline(deadline)
		defer c.nc.SetDeadline(time.Time{})
	}

	id, err := c.tp.Cmd("BODY <%s>", messageID)
	if err != nil {
		c.dead = true
		return nil, err
	}
	c.tp.StartResponse(id)
	defer c.tp.EndResponse(id)

	code, _, err := c.tp.ReadCodeLine(-1)
	if err != nil {
		c.dead = true
		return nil, err
	}
	if code != 222 {
		return nil, fmt.Errorf("nntp: BODY <%s>: %d", messageID, code)
	}
	r := c.tp.DotReader()
	data, err := io.ReadAll(r)
	if err != nil {
		c.dead = true
		return nil, err
	}
	return data, nil
}

func (c *conn) isAlive() bool { return c != nil && !c.dead }

func (c *conn) close() {
	if c == nil {
		return
	}
	if c.tp != nil {
		_, _, _ = c.cmd("QUIT")
		_ = c.tp.Close()
	} else if c.nc != nil {
		_ = c.nc.Close()
	}
}

func generateMessageID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s@ol1n.vault", hex.EncodeToString(b[:]))
}

// SplitSegments divides data into fixed-size chunks. The final chunk may be
// smaller than segSize. Returns the list of raw byte slices and the begin
// offset (1-based, inclusive) for each segment.
func SplitSegments(data []byte, segSize int) ([][]byte, []int64) {
	if segSize <= 0 {
		segSize = SegmentSize
	}
	var parts [][]byte
	var offsets []int64
	for i := 0; i < len(data); i += segSize {
		end := i + segSize
		if end > len(data) {
			end = len(data)
		}
		parts = append(parts, data[i:end])
		offsets = append(offsets, int64(i)+1)
	}
	return parts, offsets
}

// ErrNoMessageID is returned when the upstream server didn't acknowledge our
// posted article with a Message-ID we can reliably reference.
var ErrNoMessageID = errors.New("nntp: no message id")

// SanitizeMessageID strips angle brackets and trims whitespace.
func SanitizeMessageID(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	return s
}

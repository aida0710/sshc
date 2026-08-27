// Package telnet implements the client side of an interactive Telnet stream.
//
// Telnet is not raw TCP: command bytes are removed from reads, peer option
// requests are negotiated, and literal IAC bytes are escaped on writes.  The
// package intentionally does not implement authentication or command exit
// statuses; those are properties of the service running over the stream.
// Telnet provides no confidentiality or server authentication, so callers
// should make that limitation visible rather than presenting it like SSH.
package telnet

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultPort is used when Config.Address contains only a host name.
	DefaultPort = "23"
	// DefaultDialTimeout bounds a TCP connection attempt when no timeout is set.
	DefaultDialTimeout = 30 * time.Second
	// DefaultTerminalType is offered when a server requests TERMINAL-TYPE.
	DefaultTerminalType = "xterm-256color"
	// DefaultMaxSubnegotiationBytes bounds one incoming Telnet subnegotiation.
	DefaultMaxSubnegotiationBytes = 4 << 10
	// HardMaxSubnegotiationBytes prevents callers from disabling the allocation
	// bound with an accidentally excessive configuration value.
	HardMaxSubnegotiationBytes = 64 << 10

	maxAddressBytes      = 1024
	maxTerminalTypeBytes = 64
	writeChunkBytes      = 32 << 10
)

var (
	ErrInvalidAddress             = errors.New("invalid telnet address")
	ErrInvalidDialTimeout         = errors.New("invalid telnet dial timeout")
	ErrInvalidTerminalType        = errors.New("invalid telnet terminal type")
	ErrInvalidWindowSize          = errors.New("invalid telnet window size")
	ErrInvalidSubnegotiationLimit = errors.New("invalid telnet subnegotiation limit")
	ErrNilConnection              = errors.New("telnet dialer returned no connection")
	ErrMalformedNegotiation       = errors.New("malformed telnet negotiation")
	ErrSubnegotiationTooLarge     = errors.New("telnet subnegotiation exceeds limit")
)

// Config describes one unpersisted Telnet connection.
type Config struct {
	// Address is host, host:port, or a bracketed IPv6 address. Port 23 is used
	// when it is omitted.
	Address string
	// DialTimeout limits only TCP setup. The parent context owns the established
	// connection and closes it when cancelled.
	DialTimeout time.Duration
	// TerminalType is printable ASCII sent in TERMINAL-TYPE subnegotiation.
	TerminalType string
	// WindowWidth and WindowHeight are sent through NAWS. Zero values select
	// the conventional 80 by 24 default.
	WindowWidth  uint16
	WindowHeight uint16
	// MaxSubnegotiationBytes bounds a peer-controlled subnegotiation payload.
	MaxSubnegotiationBytes int
}

// Dialer opens TCP connections. DialContext is injectable for deterministic
// protocol tests and embedding in transports with their own network policy.
type Dialer struct {
	DialContext func(ctx context.Context, network, address string) (net.Conn, error)
}

// Dial opens a Telnet connection using the system network dialer.
func Dial(ctx context.Context, config Config) (*Conn, error) {
	return (Dialer{}).Dial(ctx, config)
}

// Dial opens a Telnet connection. The returned stream remains tied to ctx;
// cancelling ctx unblocks pending reads and writes by closing the TCP socket.
func (d Dialer) Dial(ctx context.Context, config Config) (*Conn, error) {
	validated, err := validate(config)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dialContext, cancel := context.WithTimeout(ctx, validated.dialTimeout)
	defer cancel()
	dial := d.DialContext
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	raw, err := dial(dialContext, "tcp", validated.address)
	if err != nil {
		if raw != nil {
			_ = raw.Close()
		}
		if cause := dialContext.Err(); cause != nil {
			return nil, cause
		}
		return nil, err
	}
	if raw == nil {
		return nil, ErrNilConnection
	}
	if cause := ctx.Err(); cause != nil {
		_ = raw.Close()
		return nil, cause
	}

	connection := &Conn{
		raw:               raw,
		reader:            bufio.NewReader(raw),
		context:           ctx,
		terminalType:      validated.terminalType,
		windowWidth:       validated.windowWidth,
		windowHeight:      validated.windowHeight,
		maxSubnegotiation: validated.maxSubnegotiation,
	}
	connection.stopContext = context.AfterFunc(ctx, connection.closeRaw)
	if cause := ctx.Err(); cause != nil {
		connection.closeRaw()
		return nil, cause
	}
	return connection, nil
}

type normalizedConfig struct {
	address           string
	dialTimeout       time.Duration
	terminalType      string
	windowWidth       uint16
	windowHeight      uint16
	maxSubnegotiation int
}

func validate(config Config) (normalizedConfig, error) {
	address, err := normalizeAddress(config.Address)
	if err != nil {
		return normalizedConfig{}, err
	}
	timeout := config.DialTimeout
	if timeout == 0 {
		timeout = DefaultDialTimeout
	}
	if timeout < 0 {
		return normalizedConfig{}, ErrInvalidDialTimeout
	}

	terminalType := config.TerminalType
	if terminalType == "" {
		terminalType = DefaultTerminalType
	}
	if len(terminalType) > maxTerminalTypeBytes {
		return normalizedConfig{}, ErrInvalidTerminalType
	}
	for index := 0; index < len(terminalType); index++ {
		if terminalType[index] < 0x21 || terminalType[index] > 0x7e {
			return normalizedConfig{}, ErrInvalidTerminalType
		}
	}

	width, height := config.WindowWidth, config.WindowHeight
	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 24
	}
	limit := config.MaxSubnegotiationBytes
	if limit == 0 {
		limit = DefaultMaxSubnegotiationBytes
	}
	if limit < 1 || limit > HardMaxSubnegotiationBytes {
		return normalizedConfig{}, ErrInvalidSubnegotiationLimit
	}
	return normalizedConfig{
		address:           address,
		dialTimeout:       timeout,
		terminalType:      terminalType,
		windowWidth:       width,
		windowHeight:      height,
		maxSubnegotiation: limit,
	}, nil
}

func normalizeAddress(address string) (string, error) {
	if address == "" || address != strings.TrimSpace(address) || len(address) > maxAddressBytes {
		return "", ErrInvalidAddress
	}
	for index := 0; index < len(address); index++ {
		if address[index] < 0x20 || address[index] == 0x7f {
			return "", ErrInvalidAddress
		}
	}
	// Address is an endpoint, not a URL. Rejecting URL delimiters here also
	// prevents accidentally treating credentials or paths as DNS names.
	if strings.ContainsAny(address, "@/\\?#") {
		return "", ErrInvalidAddress
	}
	if host, port, err := net.SplitHostPort(address); err == nil {
		if !validHost(host) || !validPort(port) {
			return "", ErrInvalidAddress
		}
		if strings.HasPrefix(address, "[") && !validIPv6Literal(host) {
			return "", ErrInvalidAddress
		}
		return net.JoinHostPort(host, port), nil
	}
	if len(address) >= 2 && address[0] == '[' && address[len(address)-1] == ']' {
		host := address[1 : len(address)-1]
		if !validIPv6Literal(host) {
			return "", ErrInvalidAddress
		}
		return net.JoinHostPort(host, DefaultPort), nil
	}
	if validIPLiteral(address) {
		return net.JoinHostPort(address, DefaultPort), nil
	}
	if !validHost(address) {
		return "", ErrInvalidAddress
	}
	return net.JoinHostPort(address, DefaultPort), nil
}

func validHost(host string) bool {
	if host == "" {
		return false
	}
	if validIPLiteral(host) {
		return true
	}
	if strings.ContainsAny(host, ":%[]") {
		return false
	}
	for index := 0; index < len(host); index++ {
		if host[index] <= 0x20 || host[index] == 0x7f {
			return false
		}
	}
	return true
}

func validIPLiteral(host string) bool {
	_, err := netip.ParseAddr(host)
	return err == nil
}

func validIPv6Literal(host string) bool {
	address, err := netip.ParseAddr(host)
	return err == nil && address.Is6()
}

func validPort(port string) bool {
	if port == "" {
		return false
	}
	for index := 0; index < len(port); index++ {
		if port[index] < '0' || port[index] > '9' {
			return false
		}
	}
	number, err := strconv.Atoi(port)
	return err == nil && number >= 1 && number <= 65535
}

// Conn is a negotiated Telnet byte stream. One concurrent reader and one or
// more concurrent writers are safe, matching net.Conn's common usage pattern.
// Telnet command sequences are never returned by Read.
type Conn struct {
	raw     net.Conn
	reader  *bufio.Reader
	context context.Context

	readMu  sync.Mutex
	writeMu sync.Mutex
	stateMu sync.Mutex

	terminalType      string
	windowWidth       uint16
	windowHeight      uint16
	maxSubnegotiation int

	localEnabled   [256]bool
	remoteEnabled  [256]bool
	localRejected  [256]bool
	remoteRejected [256]bool
	pendingReadErr error
	afterNVTCR     bool
	readTimeout    bool
	outboundNVTCR  bool

	closeOnce   sync.Once
	closeErr    error
	stopContext func() bool
}

// Read returns application data after consuming Telnet negotiation commands.
func (c *Conn) Read(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if err := c.context.Err(); err != nil {
		return 0, err
	}
	if c.pendingReadErr != nil {
		err := c.pendingReadErr
		c.pendingReadErr = nil
		return 0, err
	}

	written := 0
	for written < len(destination) {
		value, err := c.reader.ReadByte()
		if err != nil {
			return c.readResult(written, err)
		}
		if value != commandIAC {
			c.appendApplicationByte(destination, &written, value)
			if c.reader.Buffered() == 0 {
				if written > 0 {
					return written, nil
				}
			}
			continue
		}
		command, err := c.reader.ReadByte()
		if err != nil {
			return c.readResult(written, err)
		}
		switch command {
		case commandIAC:
			c.appendApplicationByte(destination, &written, commandIAC)
		case commandWILL, commandWONT, commandDO, commandDONT:
			option, optionErr := c.reader.ReadByte()
			if optionErr != nil {
				return c.readResult(written, optionErr)
			}
			if negotiationErr := c.handleNegotiation(command, option); negotiationErr != nil {
				return c.readResult(written, negotiationErr)
			}
		case commandSB:
			if subErr := c.readSubnegotiation(); subErr != nil {
				_ = c.Close()
				return c.readResult(written, subErr)
			}
		default:
			if command < commandEOF {
				_ = c.Close()
				return c.readResult(written, ErrMalformedNegotiation)
			}
			// EOF through GA are commands without option payloads. They do
			// not belong in application output.
		}
		if written > 0 && c.reader.Buffered() == 0 {
			return written, nil
		}
	}
	return written, nil
}

// appendApplicationByte applies the NVT CR rule until the peer has negotiated
// WILL BINARY. CR NUL represents a literal carriage return, so the padding NUL
// is not application data; CR LF is preserved for a raw local terminal.
func (c *Conn) appendApplicationByte(destination []byte, written *int, value byte) {
	c.stateMu.Lock()
	binary := c.remoteEnabled[optionBinary]
	c.stateMu.Unlock()
	if binary {
		c.afterNVTCR = false
		destination[*written] = value
		*written++
		return
	}
	if c.afterNVTCR {
		c.afterNVTCR = false
		if value == 0 {
			return
		}
	}
	destination[*written] = value
	*written++
	if value == '\r' {
		c.afterNVTCR = true
	}
}

func (c *Conn) readResult(written int, err error) (int, error) {
	if cause := c.context.Err(); cause != nil {
		err = cause
	}
	if c.configuredReadTimeout(err) {
		return written, nil
	}
	if written > 0 {
		c.pendingReadErr = err
		return written, nil
	}
	return 0, err
}

func (c *Conn) configuredReadTimeout(err error) bool {
	var networkError net.Error
	if !errors.As(err, &networkError) || !networkError.Timeout() {
		return false
	}
	c.stateMu.Lock()
	enabled := c.readTimeout
	c.stateMu.Unlock()
	return enabled
}

// Write sends application data, doubling each literal IAC byte as required by
// RFC 854. Large writes are chunked so escaping has a fixed memory bound.
func (c *Conn) Write(source []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.context.Err(); err != nil {
		return 0, err
	}

	written := 0
	c.stateMu.Lock()
	binary := c.localEnabled[optionBinary]
	c.stateMu.Unlock()
	for len(source) > 0 {
		count := min(len(source), writeChunkBytes)
		chunk := source[:count]
		c.stateMu.Lock()
		binary = c.localEnabled[optionBinary]
		c.stateMu.Unlock()
		application := encodeApplication(chunk, binary, &c.outboundNVTCR)
		if err := c.writeApplicationLocked(application); err != nil {
			return written, err
		}
		written += count
		source = source[count:]
	}
	return written, nil
}

// encodeApplication converts local newlines to the NVT CR LF form until the
// server has negotiated DO BINARY. Explicit CR LF and CR NUL pairs are kept,
// including when a pair crosses Write boundaries. A trailing CR is held until
// the next Write or Flush so the encoder does not have to guess prematurely.
func encodeApplication(source []byte, binary bool, pendingCR *bool) []byte {
	if binary {
		encoded := make([]byte, 0, len(source)+2)
		if *pendingCR {
			encoded = append(encoded, '\r', '\n')
			*pendingCR = false
		}
		return append(encoded, source...)
	}
	encoded := make([]byte, 0, len(source)+8)
	index := 0
	if *pendingCR && len(source) > 0 {
		encoded = append(encoded, '\r')
		if source[0] == '\n' || source[0] == 0 {
			encoded = append(encoded, source[0])
			index++
		} else {
			encoded = append(encoded, '\n')
		}
		*pendingCR = false
	}
	for ; index < len(source); index++ {
		switch source[index] {
		case '\r':
			if index+1 == len(source) {
				*pendingCR = true
				continue
			}
			encoded = append(encoded, '\r')
			if source[index+1] == '\n' || source[index+1] == 0 {
				index++
				encoded = append(encoded, source[index])
			} else {
				encoded = append(encoded, '\n')
			}
		case '\n':
			encoded = append(encoded, '\r', '\n')
		default:
			encoded = append(encoded, source[index])
		}
	}
	return encoded
}

func (c *Conn) writeApplicationLocked(application []byte) error {
	escaped := make([]byte, 0, len(application)+16)
	for _, value := range application {
		escaped = append(escaped, value)
		if value == commandIAC {
			escaped = append(escaped, commandIAC)
		}
	}
	return writeAll(c.raw, escaped)
}

// Flush resolves a trailing application CR as an NVT newline. CLI callers use
// it after each logical terminal write so pressing Enter is delivered without
// waiting for the next keystroke.
func (c *Conn) Flush() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.flushPendingLocked()
}

func (c *Conn) flushPendingLocked() error {
	if !c.outboundNVTCR {
		return nil
	}
	if err := c.writeApplicationLocked([]byte{'\r', '\n'}); err != nil {
		return err
	}
	c.outboundNVTCR = false
	return nil
}

// SetWindowSize updates the size advertised by NAWS. If the peer has already
// enabled NAWS, the new dimensions are sent immediately.
func (c *Conn) SetWindowSize(width, height uint16) error {
	if width == 0 || height == 0 {
		return ErrInvalidWindowSize
	}
	c.stateMu.Lock()
	c.windowWidth, c.windowHeight = width, height
	enabled := c.localEnabled[optionNAWS]
	c.stateMu.Unlock()
	if !enabled {
		return nil
	}
	return c.sendWindowSize(width, height)
}

// SetReadTimeout bounds a following Read. Zero restores blocking reads. It is
// used by the automation runner to verify a quiet interval after an expect.
func (c *Conn) SetReadTimeout(timeout time.Duration) error {
	if timeout < 0 {
		return errors.New("invalid Telnet read timeout")
	}
	c.stateMu.Lock()
	c.readTimeout = timeout > 0
	c.stateMu.Unlock()
	deadline := time.Time{}
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	return c.raw.SetReadDeadline(deadline)
}

// DiscardPending consumes an old banner or prompt before a scripted send so it
// cannot be mistaken for the command's response.
func (c *Conn) DiscardPending(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.SetReadTimeout(20 * time.Millisecond); err != nil {
		return err
	}
	defer func() { _ = c.SetReadTimeout(0) }()
	buffer := make([]byte, 4096)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		count, err := c.Read(buffer)
		if err != nil {
			return err
		}
		if count == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
			return nil
		}
	}
}

// Close is safe to call concurrently and more than once.
func (c *Conn) Close() error {
	if c.stopContext != nil {
		c.stopContext()
	}
	// Close must never wait for writeMu: raw.Write may be blocked indefinitely
	// when a peer stops reading. Closing the socket first is what releases that
	// writer and lets automation timeouts actually terminate the process.
	c.closeRaw()
	return c.closeErr
}

func (c *Conn) closeRaw() {
	c.closeOnce.Do(func() { c.closeErr = c.raw.Close() })
}

func (c *Conn) sendControl(payload ...byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.context.Err(); err != nil {
		return err
	}
	if err := c.flushPendingLocked(); err != nil {
		return err
	}
	return writeAll(c.raw, payload)
}

func writeAll(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		written, err := writer.Write(payload)
		if written > 0 {
			payload = payload[written:]
		}
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

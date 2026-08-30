package serialtransport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	serial "go.bug.st/serial"
)

const (
	DefaultBaudRate = 9600
	DefaultDataBits = 8
	MaxDeviceBytes  = 4096
)

var (
	ErrInvalidConfig          = errors.New("invalid serial configuration")
	ErrUnsupportedFlowControl = errors.New("serial flow control is not supported by this backend")
	ErrPlatformUnsupported    = errors.New("serial ports are not supported on this platform")
	ErrPortBusy               = errors.New("serial port is busy")
	ErrPortNotFound           = errors.New("serial port was not found")
	ErrPermissionDenied       = errors.New("permission to open the serial port was denied")
	ErrInvalidPort            = errors.New("the target is not a usable serial port")
	ErrEnumeration            = errors.New("serial ports could not be enumerated")
	ErrClosed                 = errors.New("serial stream is closed")
	ErrUnsupportedOperation   = errors.New("serial operation is not supported by this backend")
)

// Parity は、各characterへ付加するparity bitの形式である。
type Parity string

const (
	ParityNone  Parity = "none"
	ParityOdd   Parity = "odd"
	ParityEven  Parity = "even"
	ParityMark  Parity = "mark"
	ParitySpace Parity = "space"
)

// StopBits は、各characterの末尾に使うstop bit数である。
type StopBits string

const (
	StopBitsOne          StopBits = "1"
	StopBitsOnePointFive StopBits = "1.5"
	StopBitsTwo          StopBits = "2"
)

// FlowControl は、送受信を止めるhandshakeの形式である。
//
// desktop system backendはnone、RTS/CTS、XON/XOFFを実装する。別platformの
// backendが認識できる値を実装しない場合は、黙って無視せず
// ErrUnsupportedFlowControlを返さなければならない。
type FlowControl string

const (
	FlowControlNone    FlowControl = "none"
	FlowControlRTSCTS  FlowControl = "rtscts"
	FlowControlXONXOFF FlowControl = "xonxoff"
)

// Config は、一回のserial接続へ適用する設定である。Device以外のzero valueは
// Normalizeでnetwork機器consoleに一般的な8-N-1、9600 baud、flow controlなしへ補われる。
type Config struct {
	Device      string
	BaudRate    int
	DataBits    int
	Parity      Parity
	StopBits    StopBits
	FlowControl FlowControl
}

// DefaultConfig は、network機器consoleで一般的な9600 8-N-1で接続する設定を返す。
func DefaultConfig(device string) Config {
	return Config{
		Device:      device,
		BaudRate:    DefaultBaudRate,
		DataBits:    DefaultDataBits,
		Parity:      ParityNone,
		StopBits:    StopBitsOne,
		FlowControl: FlowControlNone,
	}
}

// Normalize は、省略されたoptionへ既定値を補う。Deviceは利用者が指定した識別子を
// 変形しない。
func (config Config) Normalize() Config {
	if config.BaudRate == 0 {
		config.BaudRate = DefaultBaudRate
	}
	if config.DataBits == 0 {
		config.DataBits = DefaultDataBits
	}
	if config.Parity == "" {
		config.Parity = ParityNone
	}
	if config.StopBits == "" {
		config.StopBits = StopBitsOne
	}
	if config.FlowControl == "" {
		config.FlowControl = FlowControlNone
	}
	return config
}

// Validate は、Normalize後の設定がserial driverへ渡せる範囲か検査する。
func (config Config) Validate() error {
	config = config.Normalize()
	if err := validateDevice(config.Device); err != nil {
		return err
	}
	if config.BaudRate <= 0 {
		return invalid("baud rate must be positive")
	}
	if config.DataBits < 5 || config.DataBits > 8 {
		return invalid("data bits must be between 5 and 8")
	}
	switch config.Parity {
	case ParityNone, ParityOdd, ParityEven, ParityMark, ParitySpace:
	default:
		return invalid("parity must be none, odd, even, mark, or space")
	}
	switch config.StopBits {
	case StopBitsOne, StopBitsOnePointFive, StopBitsTwo:
	default:
		return invalid("stop bits must be 1, 1.5, or 2")
	}
	switch config.FlowControl {
	case FlowControlNone, FlowControlRTSCTS, FlowControlXONXOFF:
	default:
		return invalid("flow control must be none, rtscts, or xonxoff")
	}
	return nil
}

func validateDevice(device string) error {
	if strings.TrimSpace(device) == "" {
		return invalid("device is required")
	}
	if len(device) > MaxDeviceBytes || !utf8.ValidString(device) {
		return invalid("device is not a valid identifier")
	}
	for _, character := range device {
		if character == utf8.RuneError || unicode.IsControl(character) {
			return invalid("device contains a control character")
		}
	}
	return nil
}

func invalid(detail string) error { return fmt.Errorf("%w: %s", ErrInvalidConfig, detail) }

// Device は、現在OSが列挙したserial deviceである。
type Device struct {
	Name         string
	USB          bool
	VID          string
	PID          string
	SerialNumber string
	Product      string
	Manufacturer string
}

// Mode は、device名とは独立したdriver設定である。Backend実装は認識できるが
// 実装しないFlowControlをErrUnsupportedFlowControlで拒否しなければならない。
type Mode struct {
	BaudRate    int
	DataBits    int
	Parity      Parity
	StopBits    StopBits
	FlowControl FlowControl
}

// Backend は、OS固有serial driverとの小さなseamである。製品コードはNewを使い、
// fake deviceを使うtestだけが独自backendを注入する。
type Backend interface {
	ListPorts() ([]string, error)
	OpenPort(device string, mode Mode) (io.ReadWriteCloser, error)
}

type detailedBackend interface {
	ListDevices() ([]Device, error)
}

var _ io.ReadWriteCloser = (*Stream)(nil)

// Transport はserial deviceの列挙とopenを所有する。zero valueは使えない。
type Transport struct {
	backend Backend
}

// New は、このOSのserial backendを使うTransportを返す。
func New() *Transport { return &Transport{backend: systemBackend{}} }

// NewWithBackend はtestまたは別platform integration用のbackendを注入する。
func NewWithBackend(backend Backend) (*Transport, error) {
	if backend == nil || typedNil(backend) {
		return nil, invalid("serial backend is required")
	}
	return &Transport{backend: backend}, nil
}

func typedNil(value any) bool {
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// List は、現在列挙できるdeviceを名前順で重複なく返す。空または不正な名前を
// backendが返した場合、そのentryだけを公開結果から除く。
func (transport *Transport) List(ctx context.Context) ([]Device, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if transport == nil || transport.backend == nil {
		return nil, invalid("serial transport is not initialized")
	}
	var devices []Device
	if detailed, ok := transport.backend.(detailedBackend); ok {
		listed, err := detailed.ListDevices()
		if err != nil {
			return nil, classifyBackendError("list serial ports", "", err)
		}
		devices = listed
	} else {
		names, err := transport.backend.ListPorts()
		if err != nil {
			return nil, classifyBackendError("list serial ports", "", err)
		}
		devices = make([]Device, len(names))
		for index, name := range names {
			devices[index] = Device{Name: name}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	unique := make(map[string]Device, len(devices))
	for _, device := range devices {
		if validateDevice(device.Name) == nil {
			device.VID = safeMetadata(device.VID)
			device.PID = safeMetadata(device.PID)
			device.SerialNumber = safeMetadata(device.SerialNumber)
			device.Product = safeMetadata(device.Product)
			device.Manufacturer = safeMetadata(device.Manufacturer)
			if previous, exists := unique[device.Name]; !exists || (!previous.USB && device.USB) {
				unique[device.Name] = device
			}
		}
	}
	devices = devices[:0]
	for _, device := range unique {
		devices = append(devices, device)
	}
	sort.Slice(devices, func(left, right int) bool { return devices[left].Name < devices[right].Name })
	return devices, nil
}

func safeMetadata(value string) string {
	if len(value) > 1024 || !utf8.ValidString(value) {
		return ""
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return ""
		}
	}
	return value
}

// Open は設定済みdeviceを開く。返されたStreamはctxが取り消されたときにも閉じ、
// deviceの排他openを解放する。
func (transport *Transport) Open(ctx context.Context, config Config) (*Stream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if transport == nil || transport.backend == nil {
		return nil, invalid("serial transport is not initialized")
	}
	config = config.Normalize()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	port, err := transport.backend.OpenPort(config.Device, Mode{
		BaudRate:    config.BaudRate,
		DataBits:    config.DataBits,
		Parity:      config.Parity,
		StopBits:    config.StopBits,
		FlowControl: config.FlowControl,
	})
	if err != nil {
		return nil, classifyBackendError("open serial port", config.Device, err)
	}
	if port == nil || typedNil(port) {
		return nil, fmt.Errorf("open serial port %q: backend returned no port", config.Device)
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, port.Close())
	}
	return newStream(ctx, config.Device, port), nil
}

// Stream は、開いたserial deviceの対話的なbyte streamである。同時に一つのRead、
// 一つのWrite、Closeを行える。Closeは何度呼んでもbackendを一度だけ閉じる。
type Stream struct {
	device string
	port   io.ReadWriteCloser

	closed     atomic.Bool
	closeOnce  sync.Once
	closeErr   error
	stopCancel func() bool
}

func newStream(ctx context.Context, device string, port io.ReadWriteCloser) *Stream {
	stream := &Stream{device: device, port: port}
	stream.stopCancel = context.AfterFunc(ctx, func() { _ = stream.closePort() })
	return stream
}

// Device は、このstreamが開いているdevice識別子を返す。
func (stream *Stream) Device() string { return stream.device }

func (stream *Stream) Read(buffer []byte) (int, error) {
	if stream == nil || stream.port == nil || stream.closed.Load() {
		return 0, ErrClosed
	}
	count, err := stream.port.Read(buffer)
	if err != nil {
		if stream.closed.Load() {
			err = errors.Join(ErrClosed, err)
		}
		return count, classifyBackendError("read serial port", stream.device, err)
	}
	return count, nil
}

func (stream *Stream) Write(buffer []byte) (int, error) {
	if stream == nil || stream.port == nil || stream.closed.Load() {
		return 0, ErrClosed
	}
	count, err := stream.port.Write(buffer)
	if err != nil {
		if stream.closed.Load() {
			err = errors.Join(ErrClosed, err)
		}
		return count, classifyBackendError("write serial port", stream.device, err)
	}
	return count, nil
}

type readTimeoutPort interface{ SetReadTimeout(time.Duration) error }
type dtrPort interface{ SetDTR(bool) error }
type rtsPort interface{ SetRTS(bool) error }
type breakPort interface{ Break(time.Duration) error }

// SetReadTimeout bounds a following Read. Zero restores blocking reads.
func (stream *Stream) SetReadTimeout(timeout time.Duration) error {
	if stream == nil || stream.port == nil || stream.closed.Load() {
		return ErrClosed
	}
	setter, ok := stream.port.(readTimeoutPort)
	if !ok {
		return ErrUnsupportedOperation
	}
	if timeout < 0 {
		return invalid("read timeout must not be negative")
	}
	if timeout == 0 {
		timeout = serial.NoTimeout
	}
	return setter.SetReadTimeout(timeout)
}

// DiscardPending removes stale input before a scripted send. Real serial ports
// can buffer an old prompt before the command is written; accepting that prompt
// as the command result would be a false success.
func (stream *Stream) DiscardPending(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if stream == nil || stream.port == nil || stream.closed.Load() {
		return ErrClosed
	}
	setter, ok := stream.port.(readTimeoutPort)
	if !ok {
		return nil
	}
	if err := setter.SetReadTimeout(20 * time.Millisecond); err != nil {
		return err
	}
	defer func() { _ = setter.SetReadTimeout(serial.NoTimeout) }()
	buffer := make([]byte, 4096)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		count, err := stream.port.Read(buffer)
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

// SetDTR, SetRTS, and Break expose the modem-line operations needed by
// bootloaders and embedded devices without widening the basic stream contract.
func (stream *Stream) SetDTR(enabled bool) error {
	port, ok := stream.port.(dtrPort)
	if !ok {
		return ErrUnsupportedOperation
	}
	return port.SetDTR(enabled)
}

func (stream *Stream) SetRTS(enabled bool) error {
	port, ok := stream.port.(rtsPort)
	if !ok {
		return ErrUnsupportedOperation
	}
	return port.SetRTS(enabled)
}

func (stream *Stream) Break(duration time.Duration) error {
	if duration <= 0 {
		return invalid("break duration must be positive")
	}
	port, ok := stream.port.(breakPort)
	if !ok {
		return ErrUnsupportedOperation
	}
	return port.Break(duration)
}

// Close はdeviceを解放し、待機中のRead/Writeを解除する。
func (stream *Stream) Close() error {
	if stream == nil || stream.port == nil {
		return nil
	}
	if stream.stopCancel != nil {
		stream.stopCancel()
	}
	return stream.closePort()
}

func (stream *Stream) closePort() error {
	stream.closeOnce.Do(func() {
		stream.closed.Store(true)
		stream.closeErr = stream.port.Close()
	})
	return stream.closeErr
}

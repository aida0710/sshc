package serialtransport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"

	serial "go.bug.st/serial"
)

func TestConfigDefaultsAndValidation(t *testing.T) {
	defaults := Config{Device: "/dev/ttyUSB0"}.Normalize()
	want := DefaultConfig("/dev/ttyUSB0")
	if defaults != want {
		t.Fatalf("Normalize() = %#v, want %#v", defaults, want)
	}
	if err := defaults.Validate(); err != nil {
		t.Fatalf("Validate(defaults) = %v", err)
	}

	tests := []struct {
		name   string
		change func(*Config)
	}{
		{"missing device", func(config *Config) { config.Device = "  " }},
		{"long device", func(config *Config) { config.Device = string(bytes.Repeat([]byte{'a'}, MaxDeviceBytes+1)) }},
		{"control in device", func(config *Config) { config.Device = "/dev/tty\nUSB0" }},
		{"negative baud", func(config *Config) { config.BaudRate = -1 }},
		{"few data bits", func(config *Config) { config.DataBits = 4 }},
		{"many data bits", func(config *Config) { config.DataBits = 9 }},
		{"unknown parity", func(config *Config) { config.Parity = "diagonal" }},
		{"unknown stop bits", func(config *Config) { config.StopBits = "3" }},
		{"unknown flow control", func(config *Config) { config.FlowControl = "dtrdsr" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := DefaultConfig("COM3")
			test.change(&config)
			if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Validate() = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestListReturnsSortedUniqueValidDevices(t *testing.T) {
	backend := &fakeBackend{ports: []string{"COM10", "", "COM2", "COM2", "bad\nname"}}
	transport := mustTransport(t, backend)
	devices, err := transport.List(context.Background())
	if err != nil {
		t.Fatalf("List() = %v", err)
	}
	want := []Device{{Name: "COM10"}, {Name: "COM2"}}
	if !reflect.DeepEqual(devices, want) {
		t.Fatalf("List() = %#v, want %#v", devices, want)
	}
}

func TestListHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	backend := &fakeBackend{}
	transport := mustTransport(t, backend)
	if _, err := transport.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("List() = %v, want context.Canceled", err)
	}
	if backend.listCalls != 0 {
		t.Fatalf("ListPorts calls = %d, want 0", backend.listCalls)
	}

	ctx, cancel = context.WithCancel(context.Background())
	backend.cancelList = cancel
	if _, err := transport.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("List() canceled after backend = %v, want context.Canceled", err)
	}
}

func TestOpenPassesNormalizedModeAndProvidesByteStream(t *testing.T) {
	port := newFakePort()
	port.readData = []byte("ready> ")
	backend := &fakeBackend{port: port}
	transport := mustTransport(t, backend)
	stream, err := transport.Open(context.Background(), Config{Device: "/dev/ttyACM0"})
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}
	defer stream.Close()

	wantMode := Mode{
		BaudRate:    DefaultBaudRate,
		DataBits:    DefaultDataBits,
		Parity:      ParityNone,
		StopBits:    StopBitsOne,
		FlowControl: FlowControlNone,
	}
	if backend.openDevice != "/dev/ttyACM0" || backend.openMode != wantMode {
		t.Fatalf("OpenPort = %q, %#v; want device and %#v", backend.openDevice, backend.openMode, wantMode)
	}
	if stream.Device() != "/dev/ttyACM0" {
		t.Fatalf("Device() = %q", stream.Device())
	}
	buffer := make([]byte, 32)
	count, err := stream.Read(buffer)
	if err != nil || string(buffer[:count]) != "ready> " {
		t.Fatalf("Read() = %q, %v", buffer[:count], err)
	}
	if _, err := stream.Write([]byte("show version\r")); err != nil {
		t.Fatalf("Write() = %v", err)
	}
	if got := port.written.String(); got != "show version\r" {
		t.Fatalf("written = %q", got)
	}
}

func TestOpenRejectsUnsupportedFlowBeforeOpeningDevice(t *testing.T) {
	transport := New()
	config := DefaultConfig("device-that-must-not-be-opened")
	config.FlowControl = FlowControlRTSCTS
	if _, err := transport.Open(context.Background(), config); !errors.Is(err, ErrUnsupportedFlowControl) {
		t.Fatalf("Open(rtscts) = %v, want ErrUnsupportedFlowControl", err)
	}
	config.FlowControl = FlowControlXONXOFF
	if _, err := transport.Open(context.Background(), config); !errors.Is(err, ErrUnsupportedFlowControl) {
		t.Fatalf("Open(xonxoff) = %v, want ErrUnsupportedFlowControl", err)
	}
}

func TestContextCancellationClosesPortAndUnblocksRead(t *testing.T) {
	port := newFakePort()
	port.blockRead = true
	transport := mustTransport(t, &fakeBackend{port: port})
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := transport.Open(ctx, DefaultConfig("COM7"))
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}

	readResult := make(chan error, 1)
	go func() {
		_, readErr := stream.Read(make([]byte, 1))
		readResult <- readErr
	}()
	select {
	case <-port.readStarted:
	case <-time.After(time.Second):
		t.Fatal("Read did not start")
	}
	cancel()
	select {
	case readErr := <-readResult:
		if !errors.Is(readErr, ErrClosed) {
			t.Fatalf("Read after cancellation = %v, want ErrClosed", readErr)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not unblock Read")
	}
	if calls := port.closeCalls(); calls != 1 {
		t.Fatalf("Close calls = %d, want 1", calls)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
	if calls := port.closeCalls(); calls != 1 {
		t.Fatalf("Close calls after explicit Close = %d, want 1", calls)
	}
	if _, err := stream.Write([]byte("late")); !errors.Is(err, ErrClosed) {
		t.Fatalf("Write after close = %v, want ErrClosed", err)
	}
}

func TestClosePreservesBackendErrorAndRunsOnce(t *testing.T) {
	closeErr := errors.New("driver close failed")
	port := newFakePort()
	port.closeErr = closeErr
	transport := mustTransport(t, &fakeBackend{port: port})
	stream, err := transport.Open(context.Background(), DefaultConfig("COM8"))
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := stream.Close(); !errors.Is(err, closeErr) {
			t.Fatalf("Close attempt %d = %v, want closeErr", attempt, err)
		}
	}
	if calls := port.closeCalls(); calls != 1 {
		t.Fatalf("Close calls = %d, want 1", calls)
	}
}

func TestOpenClosesPortWhenContextIsCanceledByBackend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	port := newFakePort()
	backend := &fakeBackend{port: port, cancelOpen: cancel}
	transport := mustTransport(t, backend)
	if stream, err := transport.Open(ctx, DefaultConfig("COM9")); stream != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Open() = %#v, %v; want nil, context.Canceled", stream, err)
	}
	if calls := port.closeCalls(); calls != 1 {
		t.Fatalf("Close calls = %d, want 1", calls)
	}
}

func TestDiscardPendingStopsWhenContinuousInputReachesDeadline(t *testing.T) {
	port := &continuousPort{}
	transport := mustTransport(t, &fakeBackend{port: port})
	stream, err := transport.Open(context.Background(), DefaultConfig("COM11"))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := stream.DiscardPending(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("DiscardPending() = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("discard timeout took %s", elapsed)
	}
}

func TestBackendErrorsHaveStableClassification(t *testing.T) {
	tests := []struct {
		code serial.PortErrorCode
		want error
	}{
		{serial.PortBusy, ErrPortBusy},
		{serial.PortNotFound, ErrPortNotFound},
		{serial.PermissionDenied, ErrPermissionDenied},
		{serial.InvalidSerialPort, ErrInvalidPort},
		{serial.ErrorEnumeratingPorts, ErrEnumeration},
		{serial.PortClosed, ErrClosed},
	}
	for _, test := range tests {
		err := classifyBackendError("open", "COM4", fakePortError{code: test.code})
		if !errors.Is(err, test.want) {
			t.Errorf("code %v: %v, want %v", test.code, err, test.want)
		}
	}
}

func TestStreamClassifiesDriverPortClosedError(t *testing.T) {
	port := newFakePort()
	port.readErr = fakePortError{code: serial.PortClosed}
	transport := mustTransport(t, &fakeBackend{port: port})
	stream, err := transport.Open(context.Background(), DefaultConfig("COM4"))
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}
	defer stream.Close()
	if _, err := stream.Read(make([]byte, 1)); !errors.Is(err, ErrClosed) {
		t.Fatalf("Read() = %v, want ErrClosed", err)
	}
}

func TestNilBackendAndNilPortAreRejected(t *testing.T) {
	if _, err := NewWithBackend(nil); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("NewWithBackend(nil) = %v", err)
	}
	var typedNilBackend *fakeBackend
	if _, err := NewWithBackend(typedNilBackend); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("NewWithBackend(typed nil) = %v", err)
	}
	transport := mustTransport(t, &fakeBackend{})
	if stream, err := transport.Open(context.Background(), DefaultConfig("COM5")); stream != nil || err == nil {
		t.Fatalf("Open(nil port) = %#v, %v", stream, err)
	}
	var typedNilPort *fakePort
	transport = mustTransport(t, &fakeBackend{port: typedNilPort})
	if stream, err := transport.Open(context.Background(), DefaultConfig("COM5")); stream != nil || err == nil {
		t.Fatalf("Open(typed nil port) = %#v, %v", stream, err)
	}
}

func mustTransport(t *testing.T, backend Backend) *Transport {
	t.Helper()
	transport, err := NewWithBackend(backend)
	if err != nil {
		t.Fatal(err)
	}
	return transport
}

type fakeBackend struct {
	ports      []string
	listErr    error
	listCalls  int
	cancelList context.CancelFunc

	port       io.ReadWriteCloser
	openErr    error
	openDevice string
	openMode   Mode
	cancelOpen context.CancelFunc
}

func (backend *fakeBackend) ListPorts() ([]string, error) {
	backend.listCalls++
	if backend.cancelList != nil {
		backend.cancelList()
	}
	return append([]string(nil), backend.ports...), backend.listErr
}

func (backend *fakeBackend) OpenPort(device string, mode Mode) (io.ReadWriteCloser, error) {
	backend.openDevice = device
	backend.openMode = mode
	if backend.cancelOpen != nil {
		backend.cancelOpen()
	}
	return backend.port, backend.openErr
}

type fakePort struct {
	mu          sync.Mutex
	written     bytes.Buffer
	readData    []byte
	blockRead   bool
	readStarted chan struct{}
	closed      chan struct{}
	closeOnce   sync.Once
	closes      int
	closeErr    error
	readErr     error
	writeErr    error
}

func newFakePort() *fakePort {
	return &fakePort{readStarted: make(chan struct{}), closed: make(chan struct{})}
}

func (port *fakePort) Read(buffer []byte) (int, error) {
	port.mu.Lock()
	if !port.blockRead {
		if port.readErr != nil {
			err := port.readErr
			port.mu.Unlock()
			return 0, err
		}
		count := copy(buffer, port.readData)
		port.readData = port.readData[count:]
		port.mu.Unlock()
		if count == 0 {
			return 0, io.EOF
		}
		return count, nil
	}
	port.mu.Unlock()
	select {
	case <-port.readStarted:
	default:
		close(port.readStarted)
	}
	<-port.closed
	return 0, io.ErrClosedPipe
}

func (port *fakePort) Write(buffer []byte) (int, error) {
	port.mu.Lock()
	defer port.mu.Unlock()
	select {
	case <-port.closed:
		return 0, io.ErrClosedPipe
	default:
	}
	if port.writeErr != nil {
		return 0, port.writeErr
	}
	return port.written.Write(buffer)
}

func (port *fakePort) Close() error {
	port.mu.Lock()
	port.closes++
	port.mu.Unlock()
	port.closeOnce.Do(func() { close(port.closed) })
	return port.closeErr
}

func (port *fakePort) closeCalls() int {
	port.mu.Lock()
	defer port.mu.Unlock()
	return port.closes
}

type fakePortError struct{ code serial.PortErrorCode }

func (err fakePortError) Error() string              { return "fake serial error" }
func (err fakePortError) Code() serial.PortErrorCode { return err.code }

type continuousPort struct{}

func (*continuousPort) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = 'x'
	}
	return len(buffer), nil
}
func (*continuousPort) Write(buffer []byte) (int, error)   { return len(buffer), nil }
func (*continuousPort) Close() error                       { return nil }
func (*continuousPort) SetReadTimeout(time.Duration) error { return nil }

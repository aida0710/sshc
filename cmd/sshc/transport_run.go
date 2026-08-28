package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"sshc/internal/serialtransport"
	"sshc/internal/streamrun"
	"sshc/internal/telnet"
	"sshc/internal/textencoding"
)

const (
	transportFailureExit     = 1
	transportUsageExit       = 2
	transportTimeoutExit     = 124
	transportInterruptedExit = 130
)

var telnetPlaintextWarning = "Telnet is unencrypted and does not authenticate the server; do not send secrets unless the surrounding network is trusted"

func runTransportInvocation(ctx context.Context, called transportInvocation, stdin *os.File, stdout, stderr io.Writer) int {
	return runTransportInvocationWithDependencies(ctx, called, stdin, stdout, stderr, defaultTransportDependencies())
}

type transportDependencies struct {
	listSerial func(context.Context) ([]serialtransport.Device, error)
	openSerial func(context.Context, serialtransport.Config) (duplexStream, error)
	dialTelnet func(context.Context, telnet.Config) (duplexStream, error)
}

func defaultTransportDependencies() transportDependencies {
	serialClient := serialtransport.New()
	return transportDependencies{
		listSerial: serialClient.List,
		openSerial: func(ctx context.Context, config serialtransport.Config) (duplexStream, error) {
			return serialClient.Open(ctx, config)
		},
		dialTelnet: func(ctx context.Context, config telnet.Config) (duplexStream, error) { return telnet.Dial(ctx, config) },
	}
}

func runTransportInvocationWithDependencies(ctx context.Context, called transportInvocation, stdin *os.File, stdout, stderr io.Writer, dependencies transportDependencies) int {
	if called.List {
		return runSerialList(ctx, called.JSON, stdout, stderr, dependencies)
	}
	warnings := []string(nil)
	if called.Transport == transportTelnet {
		warnings = []string{telnetPlaintextWarning}
		if !called.JSON {
			fmt.Fprintf(stderr, "sshc: warning: %s\n", telnetPlaintextWarning)
		}
	}
	if called.Run {
		return runTransportAutomation(ctx, called, warnings, stdin, stdout, stderr, dependencies)
	}

	stream, err := openTransport(ctx, called, dependencies)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", safeTransportError(called, err))
		return transportFailureExit
	}
	if err := attachTransport(ctx, stream, stdin, stdout, stderr); err != nil {
		if errors.Is(err, context.Canceled) {
			return transportInterruptedExit
		}
		fmt.Fprintf(stderr, "sshc: %v\n", safeTransportError(called, err))
		return transportFailureExit
	}
	return 0
}

type serialDeviceReport struct {
	Name         string `json:"name"`
	USB          bool   `json:"isUsb"`
	VID          string `json:"vid,omitempty"`
	PID          string `json:"pid,omitempty"`
	SerialNumber string `json:"serialNumber,omitempty"`
	Product      string `json:"product,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
}

func runSerialList(ctx context.Context, asJSON bool, stdout, stderr io.Writer, dependencies transportDependencies) int {
	devices, err := dependencies.listSerial(ctx)
	if err != nil {
		if asJSON {
			if encodeErr := json.NewEncoder(stdout).Encode(struct {
				SchemaVersion int                  `json:"schemaVersion"`
				Devices       []serialDeviceReport `json:"devices"`
				Error         string               `json:"error"`
			}{SchemaVersion: 1, Devices: []serialDeviceReport{}, Error: "serial_enumeration_failed"}); encodeErr != nil {
				fmt.Fprintln(stderr, "sshc: could not write JSON result")
			}
		} else {
			fmt.Fprintln(stderr, "sshc: serial devices could not be enumerated")
		}
		return transportFailureExit
	}
	if asJSON {
		reported := make([]serialDeviceReport, len(devices))
		for index, device := range devices {
			reported[index] = serialDeviceReport{
				Name: device.Name, USB: device.USB, VID: device.VID, PID: device.PID,
				SerialNumber: device.SerialNumber, Product: device.Product, Manufacturer: device.Manufacturer,
			}
		}
		if err := json.NewEncoder(stdout).Encode(struct {
			SchemaVersion int                  `json:"schemaVersion"`
			Devices       []serialDeviceReport `json:"devices"`
		}{SchemaVersion: 1, Devices: reported}); err != nil {
			fmt.Fprintln(stderr, "sshc: could not write JSON result")
			return transportFailureExit
		}
		return 0
	}
	for _, device := range devices {
		if _, err := fmt.Fprintln(stdout, device.Name); err != nil {
			fmt.Fprintln(stderr, "sshc: could not write serial device list")
			return transportFailureExit
		}
	}
	return 0
}

func runTransportAutomation(ctx context.Context, called transportInvocation, warnings []string, stdin io.Reader, stdout, stderr io.Writer, dependencies transportDependencies) int {
	script, err := buildTransportScript(called, stdin)
	if err != nil {
		return reportTransportSetupFailure(called, warnings, "invalid_script", err, transportUsageExit, stdout, stderr)
	}
	options := streamrun.Options{
		Timeout: called.Timeout, MaxBytes: called.MaxBytes, LookupEnv: os.LookupEnv, Settle: called.Settle,
	}
	if err := streamrun.Validate(script, options); err != nil {
		return reportTransportSetupFailure(called, warnings, "invalid_script", safeStreamRunError(err), transportUsageExit, stdout, stderr)
	}
	stream, err := openTransport(ctx, called, dependencies)
	if err != nil {
		return reportTransportSetupFailure(called, warnings, "open_failed", safeTransportError(called, err), transportFailureExit, stdout, stderr)
	}
	defer stream.Close()
	result, runErr := streamrun.Run(ctx, stream, script, options)
	transcript := streamrun.Redact(result.Transcript, result.Secrets)
	if called.JSON {
		report := newTransportRunReport(called, result, runErr, warnings)
		if err := writeTransportReport(stdout, report); err != nil {
			fmt.Fprintln(stderr, "sshc: could not write JSON result")
			return transportFailureExit
		}
	} else {
		if len(transcript) > 0 {
			if _, err := stdout.Write(transcript); err != nil {
				fmt.Fprintln(stderr, "sshc: could not write stream output")
				return transportFailureExit
			}
		}
		if runErr != nil {
			fmt.Fprintf(stderr, "sshc: %v\n", safeStreamRunError(runErr))
		}
	}
	if runErr == nil {
		return 0
	}
	var failure *streamrun.Error
	if errors.As(runErr, &failure) && failure.Kind == streamrun.FailureTimeout {
		return transportTimeoutExit
	}
	if errors.Is(runErr, context.Canceled) {
		return transportInterruptedExit
	}
	return transportFailureExit
}

func reportTransportSetupFailure(called transportInvocation, warnings []string, kind string, err error, code int, stdout, stderr io.Writer) int {
	if called.JSON {
		report := transportRunReport{
			SchemaVersion: 1, Transport: called.Transport, Target: called.Target,
			Success: false, TranscriptEncoding: "utf-8", Warnings: warnings,
			Failure: &transportRunFailure{Kind: kind, Message: err.Error()},
		}
		if writeErr := writeTransportReport(stdout, report); writeErr != nil {
			fmt.Fprintln(stderr, "sshc: could not write JSON result")
			return transportFailureExit
		}
	} else {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
	}
	return code
}

func openTransport(ctx context.Context, called transportInvocation, dependencies transportDependencies) (duplexStream, error) {
	var stream duplexStream
	var err error
	switch called.Transport {
	case transportSerial:
		stream, err = dependencies.openSerial(ctx, serialtransport.Config{
			Device: called.Target, BaudRate: called.Baud, DataBits: called.DataBits,
			Parity: serialtransport.Parity(called.Parity), StopBits: serialtransport.StopBits(called.StopBits),
			FlowControl: serialtransport.FlowControl(called.Flow),
		})
		if err != nil {
			return nil, err
		}
		if err := applySerialControls(stream, called); err != nil {
			_ = stream.Close()
			return nil, err
		}
	case transportTelnet:
		stream, err = dependencies.dialTelnet(ctx, telnet.Config{
			Address: called.Target, DialTimeout: called.ConnectTimeout, TerminalType: called.TerminalType,
		})
	default:
		return nil, errors.New("unknown transport")
	}
	if err != nil {
		return nil, err
	}
	converted, err := textencoding.Wrap(stream, called.Encoding)
	if err != nil {
		_ = stream.Close()
		return nil, err
	}
	return converted, nil
}

func applySerialControls(stream duplexStream, called transportInvocation) error {
	if called.DTR != nil {
		controller, ok := stream.(interface{ SetDTR(bool) error })
		if !ok {
			return serialtransport.ErrUnsupportedOperation
		}
		if err := controller.SetDTR(*called.DTR); err != nil {
			return err
		}
	}
	if called.RTS != nil {
		controller, ok := stream.(interface{ SetRTS(bool) error })
		if !ok {
			return serialtransport.ErrUnsupportedOperation
		}
		if err := controller.SetRTS(*called.RTS); err != nil {
			return err
		}
	}
	if called.Break > 0 {
		controller, ok := stream.(interface{ Break(time.Duration) error })
		if !ok {
			return serialtransport.ErrUnsupportedOperation
		}
		if err := controller.Break(called.Break); err != nil {
			return err
		}
	}
	return nil
}

func safeTransportError(called transportInvocation, err error) error {
	if errors.Is(err, textencoding.ErrUnsupported) {
		return errors.New("the selected text encoding is not supported")
	}
	if errors.Is(err, serialtransport.ErrUnsupportedFlowControl) {
		return errors.New("this build only supports --flow none")
	}
	if errors.Is(err, serialtransport.ErrUnsupportedOperation) {
		return errors.New("this serial backend does not support the requested modem-line operation")
	}
	if errors.Is(err, serialtransport.ErrPermissionDenied) {
		return fmt.Errorf("permission denied opening serial device %q; on Linux, check membership in the dialout group", called.Target)
	}
	if errors.Is(err, serialtransport.ErrPortBusy) {
		return fmt.Errorf("serial device %q is already in use", called.Target)
	}
	if errors.Is(err, serialtransport.ErrPortNotFound) {
		return fmt.Errorf("serial device %q was not found", called.Target)
	}
	if called.Transport == transportTelnet {
		if errors.Is(err, context.DeadlineExceeded) {
			return errors.New("Telnet connection timed out")
		}
		if errors.Is(err, telnet.ErrInvalidAddress) {
			return errors.New("Telnet target is not a valid host or host:port")
		}
	}
	if errors.Is(err, context.Canceled) {
		return errors.New("operation was interrupted")
	}
	return err
}

func safeStreamRunError(err error) error {
	var failure *streamrun.Error
	if errors.As(err, &failure) {
		return errors.New(failure.Error())
	}
	if errors.Is(err, context.Canceled) {
		return errors.New("operation was interrupted")
	}
	return errors.New("stream operation failed")
}

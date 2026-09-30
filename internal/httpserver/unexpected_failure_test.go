package httpserver

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestAnUnexpectedFailureLogsItsCauseToTheInjectedLoggerWithoutPaths(t *testing.T) {
	var logged bytes.Buffer
	engine := echo.New()
	engine.Logger = slog.New(slog.NewTextHandler(&logged, nil))
	cause := fmt.Errorf("save config: %w", &os.PathError{Op: "open", Path: "/home/someone/.ssh/config", Err: syscall.EIO})
	engine.POST("/api/v1/config/save", func(c *echo.Context) error { return serviceProblem(c, cause) })

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/config/save", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", recorder.Code)
	}
	line := logged.String()
	for _, want := range []string{"route=/api/v1/config/save", "code=internal_error", "*fs.PathError", "open <path>: input/output error"} {
		if !strings.Contains(line, want) {
			t.Errorf("log %q does not contain %q", line, want)
		}
	}
	if strings.Contains(line, "/home/someone") {
		t.Errorf("log exposed the home directory: %q", line)
	}
}

func TestAMissingCauseStillLogsWhichRouteFailed(t *testing.T) {
	var logged bytes.Buffer
	engine := echo.New()
	engine.Logger = slog.New(slog.NewTextHandler(&logged, nil))
	engine.GET("/api/v1/keys", func(c *echo.Context) error { return unexpectedProblem(c, "inventory_failed", nil) })

	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/keys", nil))

	if line := logged.String(); !strings.Contains(line, "route=/api/v1/keys") || !strings.Contains(line, "code=inventory_failed") {
		t.Fatalf("log = %q", line)
	}
}

func TestAnExpectedRefusalIsNotLogged(t *testing.T) {
	var logged bytes.Buffer
	engine := echo.New()
	engine.Logger = slog.New(slog.NewTextHandler(&logged, nil))
	engine.GET("/", func(c *echo.Context) error { return serviceProblem(c, errInvalidBody) })

	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if logged.Len() != 0 {
		t.Fatalf("a 400 was logged: %q", logged.String())
	}
}

func TestAbsolutePathsAreHiddenButOtherSlashesAreKept(t *testing.T) {
	for input, want := range map[string]string{
		"open /home/someone/.ssh/config: permission denied": "open <path>: permission denied",
		`rename C:\Users\someone\.ssh\config.tmp: denied`:   "rename <path>: denied",
		`read "/var/lib/x": EOF`:                            `read "<path>": EOF`,
		"/at/the/start failed":                              "<path> failed",
		"HTTP/1.1 via https://example.invalid/bucket":       "HTTP/1.1 via https://example.invalid/bucket",
	} {
		if got := withoutAbsolutePaths(input); got != want {
			t.Errorf("withoutAbsolutePaths(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAPathCarriedByTheErrorIsHiddenWholeEvenWhenTheHomeContainsASpace(t *testing.T) {
	denied := errors.New("access denied")
	for name, test := range map[string]struct {
		err  error
		want string
	}{
		"Windows open": {
			err:  fmt.Errorf("read config: %w", &os.PathError{Op: "open", Path: `C:\Users\Jane Doe\.ssh\config`, Err: denied}),
			want: "read config: open <path>: access denied",
		},
		"macOS rename": {
			err:  &os.LinkError{Op: "rename", Old: "/Users/Jane Doe/.ssh/config.tmp", New: "/Users/Jane Doe/.ssh/config", Err: denied},
			want: "rename <path> <path>: access denied",
		},
		"joined": {
			err:  errors.Join(errors.New("save failed"), &os.PathError{Op: "write", Path: "/Users/Jane Doe/.ssh/config", Err: denied}),
			want: "save failed\nwrite <path>: access denied",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := errorTextWithoutPaths(test.err); got != test.want {
				t.Errorf("errorTextWithoutPaths = %q, want %q", got, test.want)
			}
		})
	}
}

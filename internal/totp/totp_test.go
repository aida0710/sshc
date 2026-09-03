package totp_test

import (
	"errors"
	"testing"
	"time"

	"sshc/internal/totp"
)

func TestRFC6238Vectors(t *testing.T) {
	tests := []struct {
		algorithm string
		secret    string
		want      string
	}{
		{"SHA1", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", "94287082"},
		{"SHA256", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZA", "46119246"},
		{"SHA512", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNA", "90693936"},
	}
	for _, test := range tests {
		t.Run(test.algorithm, func(t *testing.T) {
			config, err := totp.Parse("otpauth://totp/example?secret=" + test.secret + "&algorithm=" + test.algorithm + "&digits=8&period=30")
			if err != nil {
				t.Fatal(err)
			}
			code, err := config.Code(time.Unix(59, 0))
			if err != nil || code != test.want {
				t.Fatalf("Code = %q, %v; want %q", code, err, test.want)
			}
		})
	}
}

func TestParseNormalizesASetupKeyAndURI(t *testing.T) {
	config, err := totp.Parse("jbsw y3dp-ehpk3pxp")
	if err != nil {
		t.Fatal(err)
	}
	if config.Secret != "JBSWY3DPEHPK3PXP" || config.Algorithm != "SHA1" || config.Digits != 6 || config.Period != 30 {
		t.Fatalf("config = %+v", config)
	}
	reparsed, err := totp.Parse(config.URI())
	if err != nil || reparsed != config {
		t.Fatalf("canonical round trip = %+v, %v", reparsed, err)
	}
}

func TestParseRejectsInvalidProvisioning(t *testing.T) {
	for _, value := range []string{"", "not-base32!", "otpauth://hotp/x?secret=JBSWY3DPEHPK3PXP", "otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&digits=7"} {
		if _, err := totp.Parse(value); !errors.Is(err, totp.ErrInvalid) {
			t.Errorf("Parse(%q) = %v", value, err)
		}
	}
}

func TestPromptMatchingRequiresExplicitOTPWords(t *testing.T) {
	for _, prompt := range []string{
		"Verification code: ", "OTP: ", "OTP (6 digits) for user: ", "Enter your OTP code: ",
		"One-time password: ", "Authenticator code: ",
		"One-time password (OATH) for user: ",
		"Enter the verification code shown in your authenticator app: ",
		"Please provide the code from your authenticator app: ",
		"認証コード: ", "ワンタイムパスワードを入力してください：",
	} {
		if !totp.MatchesPrompt(prompt) {
			t.Errorf("did not match %q", prompt)
		}
	}
	for _, prompt := range []string{
		"Password: ", "Passcode: ", "Code: ",
		"Password for otp-admin: ", "Passcode for totp-user: ",
		"Code for otp-user: ", "desktop token: ", "hotplug code: ",
		"Authenticator application password: ", "The verification code is unavailable: ",
	} {
		if totp.MatchesPrompt(prompt) {
			t.Errorf("matched ambiguous prompt %q", prompt)
		}
	}
}

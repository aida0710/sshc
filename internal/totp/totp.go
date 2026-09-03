// Package totp parses and generates RFC 6238 time-based one-time passwords.
// Provisioning secrets stay inside the encrypted vault; this package never
// logs or persists them.
package totp

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid TOTP provisioning secret")

const (
	defaultAlgorithm = "SHA1"
	defaultDigits    = 6
	defaultPeriod    = 30
	maxSecretBytes   = 128
)

// Config is the normalized, validated part of an otpauth TOTP URI.
type Config struct {
	Secret    string
	Algorithm string
	Digits    int
	Period    int
}

// Parse accepts either a Base32 setup key or an otpauth://totp URI.
func Parse(value string) (Config, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Config{}, ErrInvalid
	}
	config := Config{Algorithm: defaultAlgorithm, Digits: defaultDigits, Period: defaultPeriod}
	if strings.HasPrefix(strings.ToLower(value), "otpauth://") {
		parsed, err := url.Parse(value)
		if err != nil || !strings.EqualFold(parsed.Scheme, "otpauth") || !strings.EqualFold(parsed.Host, "totp") {
			return Config{}, ErrInvalid
		}
		query := parsed.Query()
		config.Secret = query.Get("secret")
		if algorithm := query.Get("algorithm"); algorithm != "" {
			config.Algorithm = strings.ToUpper(algorithm)
		}
		if digits := query.Get("digits"); digits != "" {
			parsedDigits, err := strconv.Atoi(digits)
			if err != nil {
				return Config{}, ErrInvalid
			}
			config.Digits = parsedDigits
		}
		if period := query.Get("period"); period != "" {
			parsedPeriod, err := strconv.Atoi(period)
			if err != nil {
				return Config{}, ErrInvalid
			}
			config.Period = parsedPeriod
		}
	} else {
		config.Secret = value
	}
	config.Secret = normalizeSecret(config.Secret)
	if _, err := config.secretBytes(); err != nil {
		return Config{}, err
	}
	if config.Algorithm != "SHA1" && config.Algorithm != "SHA256" && config.Algorithm != "SHA512" {
		return Config{}, ErrInvalid
	}
	if config.Digits != 6 && config.Digits != 8 {
		return Config{}, ErrInvalid
	}
	if config.Period < 5 || config.Period > 300 {
		return Config{}, ErrInvalid
	}
	return config, nil
}

func normalizeSecret(value string) string {
	value = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '\t', '\r', '\n', '=':
			return -1
		default:
			return r
		}
	}, value)
	return strings.ToUpper(value)
}

func (c Config) secretBytes() ([]byte, error) {
	if c.Secret == "" {
		return nil, ErrInvalid
	}
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(c.Secret)
	if err != nil || len(decoded) < 10 || len(decoded) > maxSecretBytes {
		return nil, ErrInvalid
	}
	return decoded, nil
}

// URI returns a canonical provisioning value suitable for encrypted storage.
func (c Config) URI() string {
	query := url.Values{}
	query.Set("algorithm", c.Algorithm)
	query.Set("digits", strconv.Itoa(c.Digits))
	query.Set("period", strconv.Itoa(c.Period))
	query.Set("secret", c.Secret)
	return "otpauth://totp/sshc?" + query.Encode()
}

// Code returns the TOTP value for at.
func (c Config) Code(at time.Time) (string, error) {
	secret, err := c.secretBytes()
	if err != nil {
		return "", err
	}
	counter := uint64(at.Unix() / int64(c.Period))
	message := make([]byte, 8)
	binary.BigEndian.PutUint64(message, counter)
	var constructor func() hash.Hash
	switch c.Algorithm {
	case "SHA1":
		constructor = sha1.New
	case "SHA256":
		constructor = sha256.New
	case "SHA512":
		constructor = sha512.New
	default:
		return "", ErrInvalid
	}
	digest := hmac.New(constructor, secret)
	_, _ = digest.Write(message)
	sum := digest.Sum(nil)
	offset := int(sum[len(sum)-1] & 0x0f)
	value := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	modulus := uint32(1_000_000)
	if c.Digits == 8 {
		modulus = 100_000_000
	}
	return fmt.Sprintf("%0*d", c.Digits, value%modulus), nil
}

var otpPrompt = regexp.MustCompile(`(?i)(one[ -]?time(?: password| code)?|verification code|authenticator(?: code)?|totp|otp|ワンタイム(?:パスワード|コード)|認証コード)`)

// MatchesPrompt deliberately recognizes only explicit OTP wording. A generic
// password or passcode question must remain interactive rather than receiving
// the second factor by mistake.
func MatchesPrompt(prompt string) bool { return otpPrompt.MatchString(prompt) }

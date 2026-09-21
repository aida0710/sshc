package remotesync

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateTargetNormalisesTheEndpointAndPath(t *testing.T) {
	target, err := ValidateTarget(TargetInput{
		Endpoint: "https://s3.example.com/", Bucket: "sshc-sync", Path: "/team/alpha/", Region: "auto",
	})
	if err != nil {
		t.Fatal(err)
	}
	if target.Endpoint != "https://s3.example.com" || target.Path != "team/alpha" {
		t.Fatalf("normalised target = %+v", target)
	}
}

func TestValidateTargetRefusesWhatWouldChangeTheSignedURL(t *testing.T) {
	valid := TargetInput{Endpoint: "https://s3.example.com", Bucket: "sshc-sync", Path: "team", Region: "auto"}
	cases := []struct {
		name  string
		input func(TargetInput) TargetInput
		want  error
	}{
		{"http endpoint", func(i TargetInput) TargetInput { i.Endpoint = "http://s3.example.com"; return i }, ErrEndpointNotHTTPS},
		{"endpoint with user", func(i TargetInput) TargetInput { i.Endpoint = "https://u@s3.example.com"; return i }, ErrEndpointNotHTTPS},
		{"endpoint with path", func(i TargetInput) TargetInput { i.Endpoint = "https://s3.example.com/x"; return i }, ErrEndpointHasPath},
		{"endpoint with query", func(i TargetInput) TargetInput { i.Endpoint = "https://s3.example.com/?x"; return i }, ErrEndpointHasPath},
		{"bucket with slash", func(i TargetInput) TargetInput { i.Bucket = "a/b"; return i }, ErrUnsafeBucketName},
		{"bucket with dots", func(i TargetInput) TargetInput { i.Bucket = ".."; return i }, ErrUnsafeBucketName},
		{"empty bucket", func(i TargetInput) TargetInput { i.Bucket = ""; return i }, ErrUnsafeBucketName},
		{"path escaping upward", func(i TargetInput) TargetInput { i.Path = "team/../other"; return i }, ErrUnsafeObjectPath},
		{"path with empty segment", func(i TargetInput) TargetInput { i.Path = "team//alpha"; return i }, ErrUnsafeObjectPath},
		{"path with query", func(i TargetInput) TargetInput { i.Path = "team?x=1"; return i }, ErrUnsafeObjectPath},
		{"overlong endpoint", func(i TargetInput) TargetInput {
			i.Endpoint = "https://" + strings.Repeat("a", MaxEndpointLength)
			return i
		}, ErrTargetTooLong},
		{"overlong path", func(i TargetInput) TargetInput { i.Path = strings.Repeat("a", MaxPathLength+1); return i }, ErrTargetTooLong},
		{"empty region", func(i TargetInput) TargetInput { i.Region = ""; return i }, errTargetRegionRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateTarget(tc.input(valid))
			if !errors.Is(err, tc.want) {
				t.Fatalf("ValidateTarget = %v, want %v", err, tc.want)
			}
		})
	}
}

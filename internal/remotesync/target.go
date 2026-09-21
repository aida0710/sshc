package remotesync

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"
)

// 接続先の入力上限。bucket と path はこの application が署名する URL の
// セグメントになり、endpoint は署名の host になる。長さも文字も、エスケープ
// ではなく拒否で守る。CLI の対話入力と HTTP の設定画面が同じ表を使う。
const (
	MaxEndpointLength = 2048
	MaxBucketLength   = 255
	MaxPathLength     = 255
	MaxRegionLength   = 64
)

// 接続先の入力が拒まれた理由。文字列は HTTP の Problem code と一致し、
// Web はそれを文言に訳す。
var (
	ErrTargetTooLong        = errors.New("invalid_request")
	ErrEndpointNotHTTPS     = errors.New("endpoint_must_be_https")
	ErrEndpointHasPath      = errors.New("endpoint_must_have_no_path")
	ErrUnsafeBucketName     = errors.New("unsafe_bucket_name")
	ErrUnsafeObjectPath     = errors.New("unsafe_object_path")
	errTargetRegionRequired = errors.New("invalid_request")
)

// TargetInput は、利用者が打った接続先。Path は前後の "/" を含んでよい。
type TargetInput struct {
	Endpoint string
	Bucket   string
	Path     string
	Region   string
}

// ValidateTarget は接続先を検証し、保存に使う正規形（endpoint は末尾の "/"
// なし、path は前後の "/" なし）を返す。
func ValidateTarget(input TargetInput) (TargetInput, error) {
	if len(input.Endpoint) == 0 || len(input.Endpoint) > MaxEndpointLength ||
		len(input.Bucket) > MaxBucketLength ||
		len(input.Path) > MaxPathLength ||
		len(input.Region) > MaxRegionLength {
		return TargetInput{}, ErrTargetTooLong
	}
	if input.Region == "" {
		return TargetInput{}, errTargetRegionRequired
	}
	parsed, err := url.Parse(input.Endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" {
		return TargetInput{}, ErrEndpointNotHTTPS
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return TargetInput{}, ErrEndpointHasPath
	}
	if !safeBucketName(input.Bucket) {
		return TargetInput{}, ErrUnsafeBucketName
	}
	path := strings.Trim(input.Path, "/")
	if !safeObjectPath(path) {
		return TargetInput{}, ErrUnsafeObjectPath
	}
	return TargetInput{
		Endpoint: strings.TrimRight(input.Endpoint, "/"), Bucket: input.Bucket, Path: path, Region: input.Region,
	}, nil
}

// safeObjectPath は bucket 名と同じくらい狭く絞ってあり、理由も同じである。
// パスはこの application が署名する URL のセグメントになるため、
// 独自のセグメントを足したり上位へ抜け出したりし得るものは、エスケープではなく拒否する。
func safeObjectPath(path string) bool {
	if path == "" {
		return true
	}
	if strings.Contains(path, "..") {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if !safeBucketName(segment) {
			return false
		}
	}
	return true
}

// safeBucketName はわざと狭く絞ってある。名前はこの application が
// 署名する URL のパスセグメントになるため、セグメントやクエリを
// 足し得るものは、エスケープではなく拒否する。
func safeBucketName(name string) bool {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
		return false
	}
	for _, character := range name {
		switch {
		case character >= 'a' && character <= 'z',
			character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9',
			character == '-', character == '.', character == '_':
		default:
			return false
		}
	}
	return filepath.Base(name) == name
}

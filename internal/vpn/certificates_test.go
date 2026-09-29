package vpn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

// testCertificateLifetime は、テストで作る証明書の有効期間である。テストの間だけ
// 使えればよい。
const testCertificateLifetime = 24 * time.Hour

// testAuthority は、テストの中で作る CA である。
type testAuthority struct {
	certificate *x509.Certificate
	key         *ecdsa.PrivateKey
	// PEM は、この CA の証明書の PEM である。
	PEM string
}

// newTestAuthority は、name を名前に持つ CA を作る。
func newTestAuthority(t *testing.T, name string) testAuthority {
	t.Helper()
	key := newTestKey(t)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(testCertificateLifetime),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return testAuthority{certificate: certificate, key: key, PEM: encodePEM("CERTIFICATE", der)}
}

// issueServerCertificate は、この CA が dnsName のサーバーへ発行した証明書と、その
// 秘密鍵（PKCS#8）を、どちらも PEM で返す。
func (authority testAuthority) issueServerCertificate(t *testing.T, dnsName string) (certificatePEM, keyPEM string) {
	t.Helper()
	key := newTestKey(t)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(testCertificateLifetime),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, authority.certificate, &key.PublicKey, authority.key)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return encodePEM("CERTIFICATE", der), encodePEM("PRIVATE KEY", private)
}

func newTestKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func encodePEM(kind string, der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}))
}

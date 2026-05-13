// internal/ghapp/jwt.go
package ghapp

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func SignAppJWT(keyPath string, appID int64, now func() time.Time) (string, error) {
	if now == nil {
		now = time.Now
	}
	key, err := loadPrivateKey(keyPath)
	if err != nil {
		return "", err
	}
	iat := now().Add(-30 * time.Second)
	claims := jwt.MapClaims{
		"iat": iat.Unix(),
		"exp": iat.Add(9 * time.Minute).Unix(),
		"iss": strconv.FormatInt(appID, 10),
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read app key: %w", err)
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("parse app key: no PEM block")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse app key: %w", err)
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("parse app key: not RSA")
	}
	return rsaKey, nil
}

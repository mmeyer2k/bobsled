// internal/ghapp/jwt_test.go
package ghapp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func writeKey(t *testing.T) (path string, pub *rsa.PublicKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	p := filepath.Join(t.TempDir(), "app.pem")
	require.NoError(t, os.WriteFile(p, pemBytes, 0o600))
	return p, &priv.PublicKey
}

func TestSignAppJWT(t *testing.T) {
	keyPath, pub := writeKey(t)
	tok, err := SignAppJWT(keyPath, 999, time.Now)
	require.NoError(t, err)

	parsed, err := jwt.Parse(tok, func(_ *jwt.Token) (any, error) { return pub, nil })
	require.NoError(t, err)
	require.True(t, parsed.Valid)

	claims := parsed.Claims.(jwt.MapClaims)
	require.Equal(t, "999", claims["iss"])
	exp := int64(claims["exp"].(float64))
	iat := int64(claims["iat"].(float64))
	require.LessOrEqual(t, exp-iat, int64(600), "JWT must expire within 10 minutes")
}

func TestSignAppJWT_BadKey(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.pem")
	require.NoError(t, os.WriteFile(bad, []byte("not a key"), 0o600))
	_, err := SignAppJWT(bad, 1, time.Now)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "parse"))
}

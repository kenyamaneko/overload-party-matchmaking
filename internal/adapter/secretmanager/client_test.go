package secretmanager_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/secretmanager"
)

// setDummyApplicationDefaultCredentials は、実クレデンシャルを持たない環境でも
// クライアント生成が Application Default Credentials の解決自体では失敗しないよう、
// 検証にしか使わないダミーのサービスアカウント鍵を GOOGLE_APPLICATION_CREDENTIALS に設定する。
func setDummyApplicationDefaultCredentials(t *testing.T) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemKey := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: mustMarshalPKCS8(t, key),
	})

	credentials := map[string]string{
		"type":         "service_account",
		"project_id":   "dummy-project",
		"private_key":  string(pemKey),
		"client_email": "dummy@dummy-project.iam.gserviceaccount.com",
		"token_uri":    "https://oauth2.googleapis.com/token",
	}
	body, err := json.Marshal(credentials)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "dummy-credentials.json")
	require.NoError(t, os.WriteFile(path, body, 0o600))
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)
}

func mustMarshalPKCS8(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return der
}

func TestNewClient(t *testing.T) {
	t.Run("Secret Managerクライアントの生成", func(t *testing.T) {
		t.Run("プロジェクトIDが空文字の状態でクライアントを生成しようとすると、生成に失敗する", func(t *testing.T) {
			_, err := secretmanager.NewClient(context.Background(), "")
			assert.Error(t, err)
		})
	})
}

func TestAccessLatest(t *testing.T) {
	t.Run("Secret Managerからの最新バージョン取得", func(t *testing.T) {
		t.Run("シークレットIDが空文字の状態で最新バージョンの取得を試みると、取得に失敗する", func(t *testing.T) {
			setDummyApplicationDefaultCredentials(t)
			c, err := secretmanager.NewClient(context.Background(), "dummy-project")
			require.NoError(t, err)

			_, err = c.AccessLatest(context.Background(), "")
			assert.Error(t, err)
		})
	})
}

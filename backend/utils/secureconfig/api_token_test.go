package secureconfig

import (
    "strings"
    "testing"

    "oneimg/backend/config"
    "crypto/sha256"
    "encoding/hex"
)

func TestAPISecretEncryptionAndBinding(t *testing.T) {
    old := config.App
    config.App = &config.Config{ConfigSecret: strings.Repeat("K", 48)}
    t.Cleanup(func() { config.App = old })

    token := "oi_test_per_user_secret"
    sum := sha256.Sum256([]byte(token))
    id := hex.EncodeToString(sum[:])
    sealed, err := EncryptAPIToken(id, token)
    if err != nil { t.Fatal(err) }
    if strings.Contains(sealed, token) || !strings.HasPrefix(sealed, "token:v1:") {
        t.Fatal("secret was not encrypted")
    }
    copy, err := DecryptAPIToken(id, sealed)
    if err != nil || copy != token { t.Fatalf("round trip failed: %v", err) }
    if _, err := DecryptAPIToken(strings.Repeat("a",64), sealed); err == nil {
        t.Fatal("ciphertext accepted under wrong token id")
    }
    if _, err := EncryptAPIToken(strings.Repeat("a",64), token); err == nil {
        t.Fatal("secret encrypted under wrong digest")
    }
    config.App.ConfigSecret = strings.Repeat("Z", 48)
    if _, err := DecryptAPIToken(id, sealed); err == nil {
        t.Fatal("ciphertext could be decrypted with a different config secret")
    }
    config.App.ConfigSecret = ""
    if _, err := DecryptAPIToken(id, sealed); err == nil {
        t.Fatal("missing config secret was allowed")
    }
}

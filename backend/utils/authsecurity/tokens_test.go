package authsecurity

import (
    "errors"
    "testing"
    "time"

    "oneimg/backend/models"

    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
)

func tokenTestDB(t *testing.T) *gorm.DB {
    t.Helper()
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    if err != nil { t.Fatal(err) }
    sqlDB, err := db.DB()
    if err != nil { t.Fatal(err) }
    sqlDB.SetMaxOpenConns(1)
    t.Cleanup(func() { _ = sqlDB.Close() })
    if err := db.AutoMigrate(&models.AuthState{}, &models.APICredential{}); err != nil { t.Fatal(err) }
    if err := db.Create(&models.AuthState{ID: 1}).Error; err != nil { t.Fatal(err) }
    return db
}

func TestMultipleCredentialsCoexistAndRevokeIndividually(t *testing.T) {
    db := tokenTestDB(t)
    expiry := time.Now().Add(7 * 24 * time.Hour)
    firstRaw, first, err := CreateNamedAPICredential(db, 1, "ScriptCat", "Upload only", []string{"upload:write"}, expiry)
    if err != nil { t.Fatal(err) }
    secondRaw, second, err := CreateNamedAPICredential(db, 1, "Read-only", "", []string{"image:read"}, expiry)
    if err != nil { t.Fatal(err) }
    if firstRaw == secondRaw || first.ID == second.ID { t.Fatal("tokens are not unique") }
    if first.ID == firstRaw { t.Fatal("bearer secret persisted as token ID") }
    cfg := models.Settings{StartAPI: true}
    if got, err := ValidateAPICredential(db, cfg, firstRaw); err != nil || !HasScope(got, "upload:write") || HasScope(got, "image:read") { t.Fatalf("first scope incorrect: %v, %v", got.Scopes, err) }
    if got, err := ValidateAPICredential(db, cfg, secondRaw); err != nil || !HasScope(got, "image:read") || HasScope(got, "upload:write") { t.Fatalf("second scope incorrect: %v, %v", got.Scopes, err) }
    if err := RevokeAPICredential(db, 1, first.ID); err != nil { t.Fatal(err) }
    if _, err := ValidateAPICredential(db, cfg, firstRaw); !errors.Is(err, ErrToken) { t.Fatal("revoked token accepted") }
    if _, err := ValidateAPICredential(db, cfg, secondRaw); err != nil { t.Fatalf("unrelated token was revoked: %v", err) }
}

func TestTokenUpdatePauseOwnershipAndExpiry(t *testing.T) {
    db := tokenTestDB(t)
    exp := time.Now().Add(30 * 24 * time.Hour)
    raw, tok, err := CreateNamedAPICredential(db, 1, "Temporary", "", []string{"upload:write"}, exp)
    if err != nil { t.Fatal(err) }
    if err := UpdateNamedAPICredential(db, 2, tok.ID, "Pwn", "", []string{"image:delete"}, exp); !errors.Is(err, ErrTokenNotFound) { t.Fatalf("cross-account update: %v", err) }
    if err := RevokeAPICredential(db, 2, tok.ID); !errors.Is(err, ErrTokenNotFound) { t.Fatalf("cross-account revoke: %v", err) }
    if err := SetAPICredentialDisabled(db, 1, tok.ID, true); err != nil { t.Fatal(err) }
    if _, err := ValidateAPICredential(db, models.Settings{StartAPI: true}, raw); !errors.Is(err, ErrToken) { t.Fatal("paused token accepted") }
    if err := SetAPICredentialDisabled(db, 1, tok.ID, false); err != nil { t.Fatal(err) }
    if err := UpdateNamedAPICredential(db, 1, tok.ID, "Read only", "", []string{"image:read"}, exp); err != nil { t.Fatal(err) }
    got, err := ValidateAPICredential(db, models.Settings{StartAPI: true}, raw)
    if err != nil || got.Name != "Read only" || got.Scopes != "image:read" { t.Fatalf("updated token incorrect: %+v, %v", got, err) }
    if err := db.Model(&models.APICredential{}).Where("id = ?", tok.ID).Update("expires_at", time.Now().Add(-time.Hour)).Error; err != nil { t.Fatal(err) }
    if _, err := ValidateAPICredential(db, models.Settings{StartAPI: true}, raw); !errors.Is(err, ErrToken) { t.Fatal("expired token accepted") }
}

func TestTokenCreationLimitsAndInvalidScopes(t *testing.T) {
    db := tokenTestDB(t)
    expiry := time.Now().Add(7 * 24 * time.Hour)
    for _, scopes := range [][]string{{}, {"setting:api"}, {"upload:write", "upload:write"}, {"*"}} {
        if _, _, _, err := ValidateTokenSpec("test", "", scopes, expiry); err == nil {
            t.Fatalf("invalid scopes accepted: %v", scopes)
        }
    }
    if _, _, _, err := ValidateTokenSpec("a\nname", "", []string{"image:read"}, expiry); err == nil { t.Fatal("control character in name accepted") }
    if _, _, _, err := ValidateTokenSpec("test", "", []string{"image:read"}, time.Now().Add(400*24*time.Hour)); err == nil { t.Fatal("expiration greater than 365 days accepted") }
    for i := 0; i < MaxActiveAPITokens; i++ {
        if _, _, err := CreateNamedAPICredential(db, 1, "test", "", []string{"upload:write"}, expiry); err != nil { t.Fatal(err) }
    }
    if _, _, err := CreateNamedAPICredential(db, 1, "overflow", "", []string{"upload:write"}, expiry); !errors.Is(err, ErrTokenLimit) { t.Fatalf("unbounded active tokens: %v", err) }
    // Independent accounts have their own active-token quota.
    if _, _, err := CreateNamedAPICredential(db, 2, "another user", "", []string{"upload:write"}, expiry); err != nil { t.Fatal(err) }
}

func TestLegacyTokenIsPreservedAndCanBeRevoked(t *testing.T) {
    db := tokenTestDB(t)
    settings := models.Settings{StartAPI: true, APIToken: "legacy-test-bearer"}
    if err := EnrollLegacyToken(db, settings); err != nil { t.Fatal(err) }
    if _, err := ValidateAPICredential(db, settings, settings.APIToken); err != nil { t.Fatal(err) }
    if _, _, err := NewAPICredential(db, 1); err != nil { t.Fatalf("compatibility generator failed: %v", err) }
    if _, err := ValidateAPICredential(db, settings, settings.APIToken); err != nil { t.Fatal("legacy was unexpectedly revoked", err) }
    if err := RevokeAPICredential(db, 1, LegacyTokenID(settings)); err != nil { t.Fatal(err) }
    if err := EnrollLegacyToken(db, settings); err != nil { t.Fatal(err) }
    if _, err := ValidateAPICredential(db, settings, settings.APIToken); !errors.Is(err, ErrToken) { t.Fatal("revoked legacy credential reactivated") }
}


func TestRotateTokenReplacesOnlySelectedSecret(t *testing.T) {
    db := tokenTestDB(t)
    expiry := time.Now().Add(30 * 24 * time.Hour)
    oldSecret, first, err := CreateNamedAPICredential(db, 1, "ScriptCat", "upload", []string{"upload:write"}, expiry)
    if err != nil { t.Fatal(err) }
    otherSecret, other, err := CreateNamedAPICredential(db, 1, "Other", "read", []string{"image:read"}, expiry)
    if err != nil { t.Fatal(err) }

    if _, _, err := RotateAPICredential(db, 2, first.ID); !errors.Is(err, ErrTokenNotFound) {
        t.Fatalf("cross-account rotation must fail: %v", err)
    }
    raw, rotated, err := RotateAPICredential(db, 1, first.ID)
    if err != nil { t.Fatal(err) }
    if raw == oldSecret || rotated.ID == first.ID {
        t.Fatal("rotation did not change credential material")
    }
    if rotated.Scopes != first.Scopes || !rotated.ExpiresAt.Equal(first.ExpiresAt) || rotated.OwnerID != first.OwnerID {
        t.Fatal("rotation changed scope, expiry or owner")
    }
    if rotated.TokenHint == "" || rotated.TokenHint == oldSecret {
        t.Fatal("rotation did not retain a safe display hint")
    }
    setting := models.Settings{StartAPI: true}
    if _, err := ValidateAPICredential(db, setting, oldSecret); !errors.Is(err, ErrToken) {
        t.Fatal("old credential remained valid")
    }
    current, err := ValidateAPICredential(db, setting, raw)
    if err != nil || current.Scopes != first.Scopes {
        t.Fatalf("rotated credential failed to authenticate: %v", err)
    }
    if _, err := ValidateAPICredential(db, setting, otherSecret); err != nil {
        t.Fatalf("unrelated token was invalidated: %v", err)
    }
    if err := RevokeAPICredential(db, 1, other.ID); err != nil { t.Fatal(err) }
    if _, _, err := RotateAPICredential(db, 1, other.ID); !errors.Is(err, ErrTokenNotFound) {
        t.Fatal("revoked token was rotated")
    }
    if _, _, err := RotateAPICredential(db, 1, first.ID); !errors.Is(err, ErrTokenNotFound) {
        t.Fatal("old management handle rotated twice")
    }
}

func TestRotateDisabledTokenRemainsDisabled(t *testing.T) {
    db := tokenTestDB(t)
    expiry := time.Now().Add(30 * 24 * time.Hour)
    _, row, err := CreateNamedAPICredential(db, 1, "Paused", "", []string{"image:read"}, expiry)
    if err != nil { t.Fatal(err) }
    if err := SetAPICredentialDisabled(db, 1, row.ID, true); err != nil { t.Fatal(err) }
    newSecret, rotated, err := RotateAPICredential(db, 1, row.ID)
    if err != nil { t.Fatal(err) }
    if !rotated.Disabled { t.Fatal("rotation unpaused a paused token") }
    if _, err := ValidateAPICredential(db, models.Settings{StartAPI: true}, newSecret); !errors.Is(err, ErrToken) {
        t.Fatal("rotation bypassed paused state")
    }
}

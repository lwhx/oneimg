package models

import "time"

// APICredential stores only a digest of the bearer secret. ID is never returned
// as an authorization credential: it is merely the opaque management handle.
// Migration adds optional metadata without changing or revoking existing tokens.
type APICredential struct {
    ID          string     `gorm:"primaryKey;size:64" json:"-"`
    OwnerID     int        `gorm:"index;not null" json:"owner_id"`
    Name        string     `gorm:"size:96;not null;default:''" json:"name"`
    Description string     `gorm:"size:500;not null;default:''" json:"description"`
    TokenHint   string     `gorm:"size:20;not null;default:''" json:"token_hint"`
    EncryptedToken string `gorm:"type:text" json:"-"` // AES-GCM under CONFIG_SECRET; never returned by list APIs
    Scopes      string     `gorm:"not null" json:"scopes"`
    ExpiresAt   time.Time  `gorm:"index;not null" json:"expires_at"`
    NeverExpires bool      `gorm:"not null;default:false" json:"never_expires"` // requires explicit selection
    LastUsedAt  *time.Time `json:"last_used_at"`
    Disabled    bool       `gorm:"not null;default:false" json:"disabled"`
    Revoked     bool       `gorm:"not null;default:false" json:"revoked"`
    RevokedAt   *time.Time `json:"revoked_at"`
    Legacy      bool       `gorm:"not null;default:false" json:"legacy"`
    CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`
    UpdatedAt   time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

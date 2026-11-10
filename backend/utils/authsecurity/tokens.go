package authsecurity

import (
    "errors"
    "fmt"
    "strings"
    "time"
    "unicode"
    "unicode/utf8"

    "oneimg/backend/models"
    "oneimg/backend/utils/secureconfig"

    "gorm.io/gorm"
    "gorm.io/gorm/clause"
)

var (
    ErrToken          = errors.New("API credential invalid or expired")
    ErrTokenLimit     = errors.New("API Token 数量已达到上限，请先撤销不再使用的 Token")
    ErrTokenNotFound  = errors.New("API Token 不存在或不可修改")
    ErrTokenImmutable = errors.New("旧版凭据不可修改，请创建新 Token 后撤销旧凭据")
)

const (
    MaxActiveAPITokens = 30
    MaxTokenLifetime   = 365 * 24 * time.Hour
    MinTokenLifetime   = 5 * time.Minute
)

type TokenScope struct {
    Code        string `json:"code"`
    Name        string `json:"name"`
    Description string `json:"description"`
}

// Only these explicit API scopes are grantable. There is deliberately no
// setting:*, storage:*, user:* or wildcard token scope.
var TokenScopes = []TokenScope{
    {"image:read", "查看图片", "读取自己的图片、详情、上传配置与标签"},
    {"upload:write", "上传图片", "上传文件或从公开 URL 导入图片"},
    {"image:delete", "删除图片", "只能删除 Token 所属账户的图片"},
    {"folder:read", "读取文件夹", "读取所属账户的文件夹列表"},
    {"folder:write", "管理文件夹", "新建、重命名、删除文件夹及移动自己的图片"},
}

func NormalizeTokenScopes(values []string) (string, error) {
    if len(values) == 0 || len(values) > len(TokenScopes) {
        return "", errors.New("必须至少选择一项有效的 API 权限")
    }
    selected := map[string]bool{}
    for _, value := range values {
        if value == "" || selected[value] {
            return "", errors.New("Token 权限不能重复或为空")
        }
        found := false
        for _, scope := range TokenScopes {
            if scope.Code == value {
                found = true
                break
            }
        }
        if !found {
            return "", fmt.Errorf("不支持的 Token 权限：%s", value)
        }
        selected[value] = true
    }
    ordered := make([]string, 0, len(selected))
    for _, scope := range TokenScopes {
        if selected[scope.Code] {
            ordered = append(ordered, scope.Code)
        }
    }
    return strings.Join(ordered, ","), nil
}

func ParseTokenScopes(value string) []string {
    if value == "" {
        return []string{}
    }
    return strings.Split(value, ",")
}

func validateTokenText(value string, max int, required bool) (string, error) {
    value = strings.TrimSpace(value)
    if (!required && value == "") || (utf8.ValidString(value) && utf8.RuneCountInString(value) <= max && (!required || value != "")) {
        for _, r := range value {
            if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
                return "", errors.New("Token 名称或描述不能包含控制字符")
            }
        }
        return value, nil
    }
    return "", fmt.Errorf("Token 名称或描述须为 %d 个字符以内的有效文本", max)
}

func ValidateTokenSpec(name, description string, scopes []string, expiry time.Time) (string, string, string, error) {
    name, err := validateTokenText(name, 64, true)
    if err != nil {
        return "", "", "", err
    }
    description, err = validateTokenText(description, 200, false)
    if err != nil {
        return "", "", "", err
    }
    codes, err := NormalizeTokenScopes(scopes)
    if err != nil {
        return "", "", "", err
    }
    now := time.Now()
    // A tiny clock skew allowance is reserved at the upper boundary for a
    // date selected in a different browser timezone.
    if expiry.IsZero() || expiry.Before(now.Add(MinTokenLifetime)) || expiry.After(now.Add(MaxTokenLifetime+time.Minute)) {
        return "", "", "", errors.New("到期时间必须在 5 分钟至 365 天之间")
    }
    return name, description, codes, nil
}

func LegacyTokenID(setting models.Settings) string {
    return Digest("legacy:" + setting.APITokenHash + ":" + setting.APIToken)
}

// Keep an existing legacy token usable until its original expiration. An
// explicit revocation is never undone by startup enrollment.
func EnrollLegacyToken(db *gorm.DB, setting models.Settings) error {
    if setting.APITokenHash == "" && setting.APIToken == "" {
        return nil
    }
    row := models.APICredential{
        ID: LegacyTokenID(setting), OwnerID: models.SuperAdminID,
        Name: "旧版 API Token", Scopes: DefaultTokenScopes,
        ExpiresAt: time.Now().Add(TokenLifetime), Legacy: true,
    }
    return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

func ValidateAPICredential(db *gorm.DB, setting models.Settings, raw string) (models.APICredential, error) {
    var token models.APICredential
    if !setting.StartAPI || raw == "" || len(raw) > 256 {
        return token, ErrToken
    }
    err := db.Where("id = ? AND legacy = ?", Digest(raw), false).First(&token).Error
    if errors.Is(err, gorm.ErrRecordNotFound) {
        if !secureconfig.CompareSecretHash(setting.APITokenHash, raw) &&
            !(setting.APIToken != "" && secureconfig.ConstantTimeEqual(setting.APIToken, raw)) {
            return token, ErrToken
        }
        err = db.First(&token, "id = ?", LegacyTokenID(setting)).Error
    }
    if err != nil {
        return token, ErrToken
    }
    now := time.Now()
    if token.Revoked || token.Disabled || !now.Before(token.ExpiresAt) || token.OwnerID <= 0 {
        return token, ErrToken
    }
    // Recheck the permission string under the atomic usage write. A scope
    // reduction or revocation racing with this request must fail closed.
    result := db.Model(&models.APICredential{}).
        Where("id = ? AND revoked = ? AND disabled = ? AND expires_at > ? AND scopes = ?", token.ID, false, false, now, token.Scopes).
        Update("last_used_at", now)
    if result.Error != nil || result.RowsAffected != 1 {
        return token, ErrToken
    }
    token.LastUsedAt = &now
    return token, nil
}

func HasScope(token models.APICredential, scope string) bool {
    for _, code := range strings.Split(token.Scopes, ",") {
        if code == scope {
            return true
        }
    }
    return false
}

// NewAPICredential is the compatibility entrypoint for the old regeneration
// route. It now ADDS a token; it never revokes other integrations.
func NewAPICredential(db *gorm.DB, owner int) (string, models.APICredential, error) {
    return CreateNamedAPICredential(db, owner, "默认 API Token", "", []string{"image:read", "upload:write"}, time.Now().Add(TokenLifetime))
}

func CreateNamedAPICredential(db *gorm.DB, owner int, name, description string, scopes []string, expiry time.Time) (string, models.APICredential, error) {
    var empty models.APICredential
    if owner <= 0 {
        return "", empty, ErrToken
    }
    name, description, codes, err := ValidateTokenSpec(name, description, scopes, expiry)
    if err != nil {
        return "", empty, err
    }
    secret, err := RandomSecret(32)
    if err != nil {
        return "", empty, err
    }
    raw := "oi_" + secret
    token := models.APICredential{
        ID: Digest(raw), OwnerID: owner, Name: name, Description: description,
        TokenHint: "oi_…" + raw[len(raw)-6:], Scopes: codes, ExpiresAt: expiry.UTC(),
    }
    err = db.Transaction(func(tx *gorm.DB) error {
        var state models.AuthState
        if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, 1).Error; err != nil {
            return err
        }
        var count int64
        if err := tx.Model(&models.APICredential{}).
            Where("owner_id = ? AND revoked = ? AND expires_at > ?", owner, false, time.Now()).
            Count(&count).Error; err != nil {
            return err
        }
        if count >= MaxActiveAPITokens {
            return ErrTokenLimit
        }
        return tx.Create(&token).Error
    })
    if err != nil {
        return "", empty, err
    }
    return raw, token, nil
}

func UpdateNamedAPICredential(db *gorm.DB, owner int, id, name, description string, scopes []string, expiry time.Time) error {
    name, description, codes, err := ValidateTokenSpec(name, description, scopes, expiry)
    if err != nil {
        return err
    }
    return db.Transaction(func(tx *gorm.DB) error {
        var token models.APICredential
        if err := tx.Where("id = ? AND owner_id = ?", id, owner).First(&token).Error; err != nil {
            if errors.Is(err, gorm.ErrRecordNotFound) { return ErrTokenNotFound }
            return err
        }
        if token.Legacy { return ErrTokenImmutable }
        if token.Revoked || !time.Now().Before(token.ExpiresAt) { return ErrTokenNotFound }
        res := tx.Model(&models.APICredential{}).
            Where("id = ? AND owner_id = ? AND revoked = ? AND expires_at > ?", id, owner, false, time.Now()).
            Updates(map[string]any{"name": name, "description": description, "scopes": codes, "expires_at": expiry.UTC(), "updated_at": time.Now()})
        if res.Error != nil { return res.Error }
        if res.RowsAffected != 1 { return ErrTokenNotFound }
        return nil
    })
}

func SetAPICredentialDisabled(db *gorm.DB, owner int, id string, disabled bool) error {
    return db.Transaction(func(tx *gorm.DB) error {
        var token models.APICredential
        if err := tx.Where("id = ? AND owner_id = ?", id, owner).First(&token).Error; err != nil {
            if errors.Is(err, gorm.ErrRecordNotFound) { return ErrTokenNotFound }
            return err
        }
        if token.Legacy { return ErrTokenImmutable }
        if token.Revoked || !time.Now().Before(token.ExpiresAt) { return ErrTokenNotFound }
        return tx.Model(&token).Updates(map[string]any{"disabled": disabled, "updated_at": time.Now()}).Error
    })
}

// RotateAPICredential atomically replaces only the selected token's secret.
// The previous digest is invalidated immediately; owner, scope and expiry stay.
// The new raw credential is returned exactly once.
func RotateAPICredential(db *gorm.DB, owner int, id string) (string, models.APICredential, error) {
    var empty models.APICredential
    if owner <= 0 {
        return "", empty, ErrTokenNotFound
    }
    secret, err := RandomSecret(32)
    if err != nil {
        return "", empty, err
    }
    raw := "oi_" + secret
    newID := Digest(raw)
    hint := "oi_…" + raw[len(raw)-6:]
    var rotated models.APICredential
    err = db.Transaction(func(tx *gorm.DB) error {
        var current models.APICredential
        if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
            Where("id = ? AND owner_id = ?", id, owner).First(&current).Error; err != nil {
            if errors.Is(err, gorm.ErrRecordNotFound) {
                return ErrTokenNotFound
            }
            return err
        }
        if current.Legacy {
            return ErrTokenImmutable
        }
        now := time.Now()
        if current.Revoked || !now.Before(current.ExpiresAt) {
            return ErrTokenNotFound
        }
        res := tx.Model(&models.APICredential{}).
            Where("id = ? AND owner_id = ? AND revoked = ? AND expires_at > ?", id, owner, false, now).
            Updates(map[string]any{
                "id": newID, "token_hint": hint, "last_used_at": nil, "updated_at": now,
            })
        if res.Error != nil {
            return res.Error
        }
        if res.RowsAffected != 1 {
            return ErrTokenNotFound
        }
        current.ID = newID
        current.TokenHint = hint
        current.LastUsedAt = nil
        current.UpdatedAt = now
        rotated = current
        return nil
    })
    if err != nil {
        return "", empty, err
    }
    return raw, rotated, nil
}


func RevokeAPICredential(db *gorm.DB, owner int, id string) error {
    now := time.Now()
    res := db.Model(&models.APICredential{}).
        Where("id = ? AND owner_id = ? AND revoked = ?", id, owner, false).
        Updates(map[string]any{"revoked": true, "revoked_at": now, "updated_at": now})
    if res.Error != nil { return res.Error }
    if res.RowsAffected == 0 { return ErrTokenNotFound }
    return nil
}

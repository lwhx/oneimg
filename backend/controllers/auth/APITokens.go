package auth

import (
    "errors"
    "net/http"
    "strconv"
    "strings"
    "time"

    "oneimg/backend/database"
    "oneimg/backend/middlewares"
    "oneimg/backend/models"
    "oneimg/backend/utils/authsecurity"
    "oneimg/backend/utils/result"

    "github.com/gin-gonic/gin"
)

// API Token management is restricted to authenticated browser sessions with
// setting:api. A bearer credential may never mint, widen or revoke credentials.
func requireTokenManager(c *gin.Context) bool {
    c.Header("Cache-Control", "private, no-store")
    if c.GetString("auth_method") == "api_token" {
        c.JSON(http.StatusForbidden, result.Error(403, "不能使用 API Token 管理其他 Token"))
        return false
    }
    user, ok := middlewares.GetCurrentUser(c)
    if !ok || user.ID <= 0 {
        c.JSON(http.StatusUnauthorized, result.Error(401, "需要有效的账户会话"))
        return false
    }
    if user.ID != models.SuperAdminID && !user.Permission.HasPermission("setting:api") {
        c.JSON(http.StatusForbidden, result.Error(403, "缺少 API 设置权限"))
        return false
    }
    return true
}

type tokenWriteRequest struct {
    Name        string    `json:"name"`
    Description string    `json:"description"`
    Scopes      []string  `json:"scopes"`
    ExpiresAt   time.Time `json:"expires_at"`
    NeverExpires bool    `json:"never_expires"`
}

func tokenPublicView(row models.APICredential) gin.H {
    state := "active"
    if row.Revoked {
        state = "revoked"
    } else if !time.Now().Before(row.ExpiresAt) {
        state = "expired"
    } else if row.Disabled {
        state = "disabled"
    }
    name := strings.TrimSpace(row.Name)
    if name == "" {
        name = "历史 API Token"
    }
    var expiry any = row.ExpiresAt
    if row.NeverExpires { expiry = nil }
    return gin.H{
        "id": row.ID, "name": name, "description": row.Description,
        "token_hint": row.TokenHint, "scopes": authsecurity.ParseTokenScopes(row.Scopes),
        "expires_at": expiry, "never_expires": row.NeverExpires,
        "can_reveal": !row.Legacy && !row.Revoked && time.Now().Before(row.ExpiresAt) && row.EncryptedToken != "",
        "last_used_at": row.LastUsedAt,
        "created_at": row.CreatedAt, "updated_at": row.UpdatedAt,
        "revoked_at": row.RevokedAt, "disabled": row.Disabled,
        "revoked": row.Revoked, "legacy": row.Legacy, "status": state,
    }
}

func tokenFailure(c *gin.Context, err error) {
    status, message := http.StatusInternalServerError, "Token 管理失败，请稍后重试"
    switch {
    case errors.Is(err, authsecurity.ErrTokenLimit), errors.Is(err, authsecurity.ErrTokenImmutable), errors.Is(err, authsecurity.ErrTokenUnrecoverable):
        status, message = http.StatusConflict, err.Error()
    case errors.Is(err, authsecurity.ErrTokenNotFound):
        status, message = http.StatusNotFound, err.Error()
    }
    c.JSON(status, result.Error(status, message))
}

func validateTokenInput(c *gin.Context, req tokenWriteRequest) bool {
    expiry := req.ExpiresAt
    if req.NeverExpires { expiry = authsecurity.PermanentTokenExpiry }
    if _, _, _, err := authsecurity.ValidateTokenSpec(req.Name, req.Description, req.Scopes, expiry); err != nil {
        c.JSON(400, result.Error(400, err.Error()))
        return false
    }
    return true
}

func ListAPITokens(c *gin.Context) {
    if !requireTokenManager(c) { return }
    limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
    if err != nil || limit < 1 || limit > 100 {
        c.JSON(400, result.Error(400, "limit 必须为 1 到 100"))
        return
    }
    page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
    if err != nil || page < 1 || page > 100000 {
        c.JSON(400, result.Error(400, "页码无效"))
        return
    }
    owner := c.GetInt("user_id")
    db := database.GetDB().DB
    var total, activeCount int64
    q := db.Model(&models.APICredential{}).Where("owner_id = ?", owner)
    if err := q.Count(&total).Error; err != nil {
        c.JSON(500, result.Error(500, "读取凭据失败")); return
    }
    if err := db.Model(&models.APICredential{}).
        Where("owner_id = ? AND revoked = ? AND expires_at > ?", owner, false, time.Now()).
        Count(&activeCount).Error; err != nil {
        c.JSON(500, result.Error(500, "读取凭据数量失败")); return
    }
    var rows []models.APICredential
    if err := q.Order("created_at desc, id desc").Limit(limit).Offset((page - 1) * limit).Find(&rows).Error; err != nil {
        c.JSON(500, result.Error(500, "查询凭据失败")); return
    }
    items := make([]gin.H, 0, len(rows))
    for _, row := range rows { items = append(items, tokenPublicView(row)) }
    c.JSON(200, result.Success("ok", gin.H{
        "tokens": items, "total": total, "page": page, "limit": limit,
        "active_count": activeCount, "max_active": authsecurity.MaxActiveAPITokens,
        "available_scopes": authsecurity.TokenScopes,
    }))
}

func CreateAPIToken(c *gin.Context) {
    if !requireTokenManager(c) { return }
    c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
    var req tokenWriteRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, result.Error(400, "Token 参数格式无效")); return
    }
    if !validateTokenInput(c, req) { return }
    raw, token, err := authsecurity.CreateManagedAPICredential(database.GetDB().DB, c.GetInt("user_id"), req.Name, req.Description, req.Scopes, req.ExpiresAt, req.NeverExpires)
    if err != nil {
        tokenFailure(c, err)
        return
    }
    c.JSON(http.StatusCreated, result.Success("Token 已创建，可随时在后台重新复制", gin.H{
        "token": raw, "api_token": raw, "credential": tokenPublicView(token),
    }))
}

func validManagedTokenID(id string) bool {
    if len(id) != 64 { return false }
    for _, ch := range id {
        if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') { return false }
    }
    return true
}

func UpdateAPIToken(c *gin.Context) {
    if !requireTokenManager(c) { return }
    id := c.Param("id")
    if !validManagedTokenID(id) { c.JSON(400, result.Error(400, "Token ID 无效")); return }
    c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
    var req tokenWriteRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, result.Error(400, "Token 参数格式无效")); return
    }
    if !validateTokenInput(c, req) { return }
    if err := authsecurity.UpdateManagedAPICredential(database.GetDB().DB, c.GetInt("user_id"), id, req.Name, req.Description, req.Scopes, req.ExpiresAt, req.NeverExpires); err != nil {
        tokenFailure(c, err); return
    }
    c.JSON(200, result.Success("Token 设置已更新", nil))
}

func SetAPITokenStatus(c *gin.Context) {
    if !requireTokenManager(c) { return }
    id := c.Param("id")
    if !validManagedTokenID(id) { c.JSON(400, result.Error(400, "Token ID 无效")); return }
    c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
    var req struct { Disabled *bool `json:"disabled"` }
    if err := c.ShouldBindJSON(&req); err != nil || req.Disabled == nil {
        c.JSON(400, result.Error(400, "必须指定 disabled 布尔值")); return
    }
    if err := authsecurity.SetAPICredentialDisabled(database.GetDB().DB, c.GetInt("user_id"), id, *req.Disabled); err != nil {
        tokenFailure(c, err); return
    }
    c.JSON(200, result.Success("Token 状态已更新", nil))
}

// RotateAPIToken replaces one token while preserving its scope and expiry.
// Management requires a signed-in session; bearer tokens cannot call it.
func RotateAPIToken(c *gin.Context) {
    if !requireTokenManager(c) {
        return
    }
    id := c.Param("id")
    if !validManagedTokenID(id) {
        c.JSON(400, result.Error(400, "Token ID 无效"))
        return
    }
    raw, rotated, err := authsecurity.RotateAPICredential(database.GetDB().DB, c.GetInt("user_id"), id)
    if err != nil {
        tokenFailure(c, err)
        return
    }
    c.JSON(http.StatusOK, result.Success("Token 已重新生成，旧 Token 已失效，请立即复制新密钥", gin.H{
        "token": raw,
        "api_token": raw,
        "credential": tokenPublicView(rotated),
    }))
}



// RevealAPIToken returns the SAME bearer secret, not a replacement.
// Only an authorized browser session belonging to the owner may invoke it.
func RevealAPIToken(c *gin.Context) {
    if !requireTokenManager(c) { return }
    c.Header("Cache-Control", "no-store")
    c.Header("Pragma", "no-cache")
    id := c.Param("id")
    if !validManagedTokenID(id) {
        c.JSON(400, result.Error(400, "Token ID 无效"))
        return
    }
    raw, err := authsecurity.RevealAPICredential(database.GetDB().DB, c.GetInt("user_id"), id)
    if err != nil {
        tokenFailure(c, err)
        return
    }
    c.JSON(http.StatusOK, result.Success("已读取原 API Token，密钥保持不变", gin.H{
        "token": raw, "api_token": raw,
    }))
}

func RevokeAPIToken(c *gin.Context) {
    if !requireTokenManager(c) { return }
    id := c.Param("id")
    if !validManagedTokenID(id) { c.JSON(400, result.Error(400, "Token ID 无效")); return }
    if err := authsecurity.RevokeAPICredential(database.GetDB().DB, c.GetInt("user_id"), id); err != nil {
        tokenFailure(c, err); return
    }
    c.JSON(200, result.Success("Token 已永久撤销", nil))
}

package middlewares

import (
    "testing"

    "oneimg/backend/models"
)

func TestTokenRouteAllowlist(t *testing.T) {
    cases := []struct {
        scopes string
        method string
        path string
        allowed bool
    }{
        {"upload:write", "GET", "/api/uploadConfig", true},
        {"upload:write", "POST", "/api/upload/images", true},
        {"upload:write", "POST", "/api/images/url", true},
        {"upload:write", "GET", "/api/images", false},
        {"image:read", "GET", "/api/images/12", true},
        {"image:read", "DELETE", "/api/images/12", false},
        {"image:delete", "DELETE", "/api/images/12", true},
        {"image:delete", "GET", "/api/images/12", false},
        {"image:delete", "DELETE", "/api/images/12/more", false},
        {"folder:read", "GET", "/api/folders", true},
        {"folder:read", "POST", "/api/folders", false},
        {"folder:write", "GET", "/api/folders", true},
        {"folder:write", "PUT", "/api/folders/2", true},
        {"folder:write", "DELETE", "/api/folders/2", true},
        {"folder:write", "PUT", "/api/images/folder", true},
        {"folder:write", "DELETE", "/api/images/3", false},
        {"image:read,upload:write,image:delete,folder:read,folder:write", "GET", "/api/settings/get", false},
        {"image:read,upload:write,image:delete,folder:read,folder:write", "POST", "/api/tokens", false},
        {"image:read,upload:write,image:delete,folder:read,folder:write", "DELETE", "/api/tokens/abc", false},
        {"image:read,upload:write,image:delete,folder:read,folder:write", "GET", "/api/user/status", true},
        {"upload:write", "POST", "/api/images/upload", false},
    }
    for _, tc := range cases {
        tok := models.APICredential{Scopes: tc.scopes}
        if got := tokenRouteAllowed(tc.method, tc.path, tok); got != tc.allowed {
            t.Errorf("%q %s %s: got %v, want %v", tc.scopes, tc.method, tc.path, got, tc.allowed)
        }
    }
}

package images

import (
    "net/http/httptest"
    "testing"

    "oneimg/backend/models"

    "github.com/gin-gonic/gin"
)

func TestAPITokenDoesNotInheritSuperAdminImageBypass(t *testing.T) {
    c, _ := gin.CreateTestContext(httptest.NewRecorder())
    c.Set("user_id", models.SuperAdminID)
    c.Set("user_role", models.RoleUser)
    c.Set("auth_method", "api_token")

    if CheckImageAccessPermission(c, models.Image{UserId: 2}, "") {
        t.Fatal("superadmin-owned API token can read other users' images")
    }
    if CheckImageAccessPermission(c, models.Image{UserId: 2}, "image:delete") {
        t.Fatal("superadmin-owned API token can delete other users' images")
    }
    if !CheckImageAccessPermission(c, models.Image{UserId: models.SuperAdminID}, "image:delete") {
        t.Fatal("API token must retain access to images owned by its own user")
    }
}

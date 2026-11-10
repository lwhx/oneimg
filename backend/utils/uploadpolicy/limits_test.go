package uploadpolicy

import (
    "testing"

    "oneimg/backend/config"
    "oneimg/backend/models"
)

func TestUploadBounds(t *testing.T) {
    if got := MaxFiles(models.Settings{MaxUploadFiles: 100}); got != config.HardMaxUploadFiles {
        t.Fatalf("MaxFiles(100) = %d, want %d", got, config.HardMaxUploadFiles)
    }
    if got := MaxFiles(models.Settings{MaxUploadFiles: 101}); got != config.DefaultMaxUploadFiles {
        t.Fatalf("out-of-bounds MaxFiles = %d, want default %d", got, config.DefaultMaxUploadFiles)
    }
    if err := ValidateCount(models.Settings{MaxUploadFiles: 100}, 100); err != nil {
        t.Fatalf("100 files must be accepted: %v", err)
    }
    if err := ValidateCount(models.Settings{MaxUploadFiles: 100}, 101); err == nil {
        t.Fatal("101 files must be rejected")
    }
}

func TestUploadRequestByteCap(t *testing.T) {
    cases := []struct {
        name string
        setting models.Settings
        want int64
    }{
        {"small configured batch", models.Settings{MaxUploadFiles: 2, MaxFileSize: 20 << 20}, 41 << 20},
        {"large batch is bounded", models.Settings{MaxUploadFiles: 100, MaxFileSize: 64 << 20}, (256 + 1) << 20},
        {"invalid file size remains bounded", models.Settings{MaxUploadFiles: 100, MaxFileSize: 256 << 20}, (256 + 1) << 20},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            if got := RequestBytes(tc.setting); got != tc.want {
                t.Fatalf("RequestBytes = %d, want %d", got, tc.want)
            }
        })
    }
}

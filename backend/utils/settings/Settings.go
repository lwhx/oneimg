package settings

import (
	"oneimg/backend/database"
	"oneimg/backend/models"
	"oneimg/backend/utils/secureconfig"
)

func GetSettings() (models.Settings, error) {
	// 获取数据库实例
	db := database.GetDB()
	if db == nil {
		return models.Settings{}, nil
	}
	var settings models.Settings
	err := db.DB.First(&settings).Error
	if err != nil {
		return settings, err
	}

	if changed, migrateErr := secureconfig.TryMigrateSettingsSecrets(&settings); migrateErr == nil && changed {
		_ = db.DB.Model(&settings).Updates(map[string]any{
			"tg_bot_token":         settings.TGBotToken,
			"api_token":            settings.APIToken,
			"api_token_hash":       settings.APITokenHash,
			"oidc_client_secret":   settings.OIDCClientSecret,
			"turnstile_secret_key": settings.TurnstileSecret,
			"cloudflare_api_token": settings.CloudflareAPIToken,
		}).Error
	}

	if settings.TGBotToken != "" {
		decrypted, decryptErr := secureconfig.DecryptSettingValue("tg_bot_token", settings.TGBotToken)
		if decryptErr != nil {
			return settings, decryptErr
		}
		settings.TGBotToken = decrypted
	}

	if settings.OIDCClientSecret != "" {
		decrypted, decryptErr := secureconfig.DecryptSettingValue("oidc_client_secret", settings.OIDCClientSecret)
		if decryptErr != nil {
			return settings, decryptErr
		}
		settings.OIDCClientSecret = decrypted
	}

	if settings.TurnstileSecret != "" {
		decrypted, decryptErr := secureconfig.DecryptSettingValue("turnstile_secret_key", settings.TurnstileSecret)
		if decryptErr != nil {
			return settings, decryptErr
		}
		settings.TurnstileSecret = decrypted
	}

	if settings.CloudflareAPIToken != "" {
		decrypted, decryptErr := secureconfig.DecryptSettingValue("cloudflare_api_token", settings.CloudflareAPIToken)
		if decryptErr != nil {
			return settings, decryptErr
		}
		settings.CloudflareAPIToken = decrypted
	}

	return settings, nil
}

package config

// Hard ceilings protect resources and match persistent column constraints.
const (
	DefaultMaxUploadFiles          = 20
	HardMaxUploadFiles             = 100
	DefaultMaxUploadBytes    int64 = 20 << 20
	HardMaxUploadBytes       int64 = 64 << 20
	HardMaxUploadRequestBytes int64 = 256 << 20
	HardMaxImageDimension          = 16000
	HardMaxImagePixels       int64 = 24_000_000
	DefaultTagMaxLength            = 10
	HardTagMaxLength               = 50
	DefaultRandomImageLimit        = 20
	HardRandomImageLimit           = 100
)

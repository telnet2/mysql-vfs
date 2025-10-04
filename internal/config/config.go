package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultConfigPath         = "config/config.yaml"
	defaultBucketURL          = "file:///tmp/vfs-blob"
	defaultInlineJSONMaxBytes = int64(1 << 20) // 1 MiB
)

type MySQL struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	Params   string `yaml:"params"`
}

type Service struct {
	Address string `yaml:"address"`
}

type Blob struct {
	BucketURL string `yaml:"bucket_url"`
}

type Storage struct {
	InlineJSONMaxBytes   int64    `yaml:"inline_json_max_bytes"`
	InlineJSONMediaTypes []string `yaml:"inline_json_media_types"`
}

type Cron struct {
	SchedulerInterval time.Duration
	LockTimeout       time.Duration
}

type Webhook struct {
	CallbackSecret string `yaml:"callback_secret"`
	BaseURL        string `yaml:"base_url"`
}

type Settings struct {
	Services map[string]Service `yaml:"services"`
	MySQL    MySQL              `yaml:"mysql"`
	Blob     Blob               `yaml:"blob"`
	Storage  Storage            `yaml:"storage"`
	Webhook  Webhook            `yaml:"webhook"`
	Cron     Cron               `yaml:"cron"`
}

type rawSettings struct {
	Services map[string]Service `yaml:"services"`
	MySQL    MySQL              `yaml:"mysql"`
	Blob     Blob               `yaml:"blob"`
	Storage  Storage            `yaml:"storage"`
	Webhook  Webhook            `yaml:"webhook"`
	Cron     struct {
		SchedulerInterval string `yaml:"scheduler_interval"`
		LockTimeout       string `yaml:"lock_timeout"`
	} `yaml:"cron"`
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Load() Settings {
	path := getenv("VFS_CONFIG_PATH", defaultConfigPath)
	if cfg, err := loadFromFile(path); err == nil {
		cfg.applyDefaults()
		return cfg
	}
	cfg := loadFromEnv()
	cfg.applyDefaults()
	return cfg
}

func loadFromFile(path string) (Settings, error) {
	var empty Settings
	clean := filepath.Clean(path)
	data, err := os.ReadFile(clean)
	if err != nil {
		return empty, err
	}
	var raw rawSettings
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return empty, err
	}
	cfg := Settings{
		Services: raw.Services,
		MySQL:    raw.MySQL,
		Blob:     raw.Blob,
		Storage:  raw.Storage,
		Webhook:  raw.Webhook,
	}
	if cfg.Services == nil {
		cfg.Services = map[string]Service{}
	}
	if raw.Cron.SchedulerInterval != "" {
		d, err := time.ParseDuration(raw.Cron.SchedulerInterval)
		if err != nil {
			return empty, err
		}
		cfg.Cron.SchedulerInterval = d
	}
	if raw.Cron.LockTimeout != "" {
		d, err := time.ParseDuration(raw.Cron.LockTimeout)
		if err != nil {
			return empty, err
		}
		cfg.Cron.LockTimeout = d
	}
	return cfg, nil
}

func loadFromEnv() Settings {
	port, _ := strconv.Atoi(getenv("MYSQL_PORT", "3306"))
	inlineMax, err := strconv.ParseInt(getenv("STORAGE_INLINE_JSON_MAX_BYTES", "0"), 10, 64)
	if err != nil || inlineMax <= 0 {
		inlineMax = defaultInlineJSONMaxBytes
	}
	mediaTypes := getenv("STORAGE_INLINE_JSON_MEDIA_TYPES", "application/json,text/json")
	types := strings.Split(mediaTypes, ",")
	result := make([]string, 0, len(types))
	for _, t := range types {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		result = []string{"application/json"}
	}
	cronInterval, _ := time.ParseDuration(getenv("CRON_SCHEDULER_INTERVAL", "5s"))
	if cronInterval <= 0 {
		cronInterval = 5 * time.Second
	}
	lockTimeout, _ := time.ParseDuration(getenv("CRON_LOCK_TIMEOUT", "30s"))
	if lockTimeout <= 0 {
		lockTimeout = 30 * time.Second
	}
	services := map[string]Service{
		"metadata":  {Address: getenv("METADATA_SERVER_ADDR", ":8081")},
		"content":   {Address: getenv("CONTENT_SERVER_ADDR", ":8082")},
		"scheduler": {Address: getenv("SCHEDULER_SERVER_ADDR", ":8083")},
		"webhook":   {Address: getenv("WEBHOOK_SERVER_ADDR", ":8084")},
	}
	return Settings{
		Services: services,
		MySQL: MySQL{
			User:     getenv("MYSQL_USER", "root"),
			Password: getenv("MYSQL_PASSWORD", ""),
			Host:     getenv("MYSQL_HOST", "mysql"),
			Port:     port,
			Database: getenv("MYSQL_DATABASE", "vfs"),
			Params:   getenv("MYSQL_PARAMS", "charset=utf8mb4&parseTime=True&loc=Local"),
		},
		Blob: Blob{BucketURL: getenv("BLOB_BUCKET_URL", defaultBucketURL)},
		Storage: Storage{
			InlineJSONMaxBytes:   inlineMax,
			InlineJSONMediaTypes: result,
		},
		Webhook: Webhook{
			CallbackSecret: getenv("WEBHOOK_SECRET", ""),
			BaseURL:        getenv("WEBHOOK_BASE_URL", ""),
		},
		Cron: Cron{
			SchedulerInterval: cronInterval,
			LockTimeout:       lockTimeout,
		},
	}
}

func (s *Settings) applyDefaults() {
	if s.Services == nil {
		s.Services = map[string]Service{}
	}
	for name, svc := range s.Services {
		s.Services[name] = Service{Address: strings.TrimSpace(svc.Address)}
	}
	if s.MySQL.Host == "" {
		s.MySQL.Host = "mysql"
	}
	if s.MySQL.Port == 0 {
		s.MySQL.Port = 3306
	}
	if s.MySQL.Database == "" {
		s.MySQL.Database = "vfs"
	}
	if s.MySQL.Params == "" {
		s.MySQL.Params = "charset=utf8mb4&parseTime=True&loc=Local"
	}
	if s.Blob.BucketURL == "" {
		s.Blob.BucketURL = defaultBucketURL
	}
	if s.Storage.InlineJSONMaxBytes <= 0 {
		s.Storage.InlineJSONMaxBytes = defaultInlineJSONMaxBytes
	}
	if len(s.Storage.InlineJSONMediaTypes) == 0 {
		s.Storage.InlineJSONMediaTypes = []string{"application/json", "text/json"}
	}
	if s.Cron.SchedulerInterval <= 0 {
		s.Cron.SchedulerInterval = 5 * time.Second
	}
	if s.Cron.LockTimeout <= 0 {
		s.Cron.LockTimeout = 30 * time.Second
	}
}

func (s Settings) ServiceAddress(name string) string {
	if svc, ok := s.Services[name]; ok && strings.TrimSpace(svc.Address) != "" {
		return strings.TrimSpace(svc.Address)
	}
	if svc, ok := s.Services["default"]; ok && strings.TrimSpace(svc.Address) != "" {
		return strings.TrimSpace(svc.Address)
	}
	return ""
}

func (s Settings) InlineJSONMaxBytes() int64 {
	if s.Storage.InlineJSONMaxBytes > 0 {
		return s.Storage.InlineJSONMaxBytes
	}
	return defaultInlineJSONMaxBytes
}

func (s Settings) InlineJSONMediaTypes() []string {
	media := s.Storage.InlineJSONMediaTypes
	if len(media) == 0 {
		media = []string{"application/json", "text/json"}
	}
	copyOf := make([]string, len(media))
	for i, v := range media {
		copyOf[i] = strings.TrimSpace(v)
	}
	return copyOf
}

func (s Settings) BlobBucketURL() string {
	if strings.TrimSpace(s.Blob.BucketURL) == "" {
		return defaultBucketURL
	}
	return strings.TrimSpace(s.Blob.BucketURL)
}

func (s Settings) CronSchedulerInterval() time.Duration {
	if s.Cron.SchedulerInterval > 0 {
		return s.Cron.SchedulerInterval
	}
	return 5 * time.Second
}

func (s Settings) CronLockTimeout() time.Duration {
	if s.Cron.LockTimeout > 0 {
		return s.Cron.LockTimeout
	}
	return 30 * time.Second
}

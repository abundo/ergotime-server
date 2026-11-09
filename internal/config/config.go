package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds application configuration.
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Auth      AuthConfig      `mapstructure:"auth"`
	CORS      CORSConfig      `mapstructure:"cors"`
	Security  SecurityConfig  `mapstructure:"security"`
	Bootstrap BootstrapConfig `mapstructure:"bootstrap"`
	Static    StaticConfig    `mapstructure:"static"`
}

type ServerConfig struct {
	Host      string `mapstructure:"host"`
	Port      int    `mapstructure:"port"`
	PublicURL string `mapstructure:"public_url"`
	Mode      string `mapstructure:"mode"`
}

type DatabaseConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	User         string `mapstructure:"user"`
	Password     string `mapstructure:"password"`
	Name         string `mapstructure:"name"`
	SSLMode      string `mapstructure:"sslmode"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

type AuthConfig struct {
	JWTSecret      string `mapstructure:"jwt_secret"`
	TokenTTLHours  int    `mapstructure:"token_ttl_hours"`
	CookieName     string `mapstructure:"cookie_name"`
	CookieSecure   bool   `mapstructure:"cookie_secure"`
	CookieSameSite string `mapstructure:"cookie_samesite"`
	CookieDomain   string `mapstructure:"cookie_domain"`
	CookiePath     string `mapstructure:"cookie_path"`
}

func (a AuthConfig) TokenTTL() time.Duration {
	h := a.TokenTTLHours
	if h <= 0 {
		h = 24
	}
	return time.Duration(h) * time.Hour
}

type CORSConfig struct {
	AllowedOrigins   []string `mapstructure:"allowed_origins"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
}

type SecurityConfig struct {
	CSP            string `mapstructure:"csp"`
	LoginRateLimit int    `mapstructure:"login_rate_limit"`
}

type BootstrapConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// StaticConfig controls SPA serving.
//
// Dir, when set and present on disk, overrides the assets embedded in the binary
// (useful while iterating on the frontend without recompiling Go). Leave empty
// in production to serve the embedded Vue build.
type StaticConfig struct {
	Dir string `mapstructure:"dir"`
	SPA bool   `mapstructure:"spa"`
}

// Load reads configuration from file and environment variables.
// Env vars use prefix ERGOTIME_ and nested keys with underscore, e.g. ERGOTIME_DATABASE_HOST.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")

	setDefaults(v)

	if path != "" {
		v.SetConfigFile(path)
	} else if p := os.Getenv("ERGOTIME_CONFIG"); p != "" {
		v.SetConfigFile(p)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
		v.AddConfigPath("/etc/ergotime")
	}

	v.SetEnvPrefix("ERGOTIME")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// only ignore "not found" when searching default paths
			if path != "" || os.Getenv("ERGOTIME_CONFIG") != "" {
				return nil, fmt.Errorf("read config: %w", err)
			}
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "127.0.0.1")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "release")
	v.SetDefault("database.host", "127.0.0.1")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "ergotime")
	v.SetDefault("database.name", "ergotime")
	v.SetDefault("database.sslmode", "disable")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("auth.token_ttl_hours", 24)
	v.SetDefault("auth.cookie_name", "ergotime_session")
	v.SetDefault("auth.cookie_secure", true)
	v.SetDefault("auth.cookie_samesite", "lax")
	v.SetDefault("auth.cookie_path", "/")
	v.SetDefault("cors.allow_credentials", true)
	v.SetDefault("security.login_rate_limit", 10)
	v.SetDefault("bootstrap.enabled", true)
	v.SetDefault("bootstrap.username", "admin")
	v.SetDefault("bootstrap.password", "admin")
	// Empty dir → serve SPA assets embedded in the binary (make frontend && make server).
	v.SetDefault("static.dir", "")
	v.SetDefault("static.spa", true)
}

// Validate checks required settings.
func (c *Config) Validate() error {
	if c.Auth.JWTSecret == "" || c.Auth.JWTSecret == "CHANGE-ME-TO-A-LONG-RANDOM-SECRET" {
		if c.Server.Mode == "release" {
			return fmt.Errorf("auth.jwt_secret must be set to a strong random value in release mode")
		}
		// allow weak secret in debug for local development
		if c.Auth.JWTSecret == "" {
			c.Auth.JWTSecret = "dev-only-insecure-secret-change-me"
		}
	}
	if c.Database.Password == "" && c.Server.Mode == "release" {
		return fmt.Errorf("database.password must be set")
	}
	return nil
}

func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (c *Config) IsDebug() bool {
	return strings.EqualFold(c.Server.Mode, "debug")
}

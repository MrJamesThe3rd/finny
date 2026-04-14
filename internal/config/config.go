package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	App struct {
		Name string `envconfig:"APP_NAME" default:"Finny"`
		Port int    `envconfig:"PORT" default:"8080"`
	}

	DB struct {
		Host     string `envconfig:"DB_HOST" default:"localhost"`
		Port     int    `envconfig:"DB_PORT" default:"5432"`
		User     string `envconfig:"DB_USER" default:"postgres"`
		Password string `envconfig:"DB_PASSWORD" default:""`
		Name     string `envconfig:"DB_NAME" default:"finny"`
		// Helper to construct connection string if needed, or we can build it in main
	}

	Server struct {
		Timeout time.Duration `envconfig:"SERVER_TIMEOUT" default:"30s"`
	}

	Paperless struct {
		BaseURL string `envconfig:"PAPERLESS_BASE_URL"`
		Token   string `envconfig:"PAPERLESS_TOKEN"`
	}

	Auth struct {
		JWTSecret          string        `envconfig:"AUTH_JWTSECRET"          required:"true"`
		AccessTokenExpiry  time.Duration `envconfig:"AUTH_ACCESSTOKENEXPIRY"  default:"15m"`
		RefreshTokenExpiry time.Duration `envconfig:"AUTH_REFRESHTOKENEXPIRY" default:"168h"`
		CORSAllowedOrigin  string        `envconfig:"AUTH_CORSALLOWEDORIGIN"  default:"http://localhost:5173"`
	}
}

func (c *Config) ConnectionString() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		c.DB.User, c.DB.Password, c.DB.Host, c.DB.Port, c.DB.Name)
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to process config: %w", err)
	}

	return &cfg, nil
}

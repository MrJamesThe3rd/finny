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
		Token string `envconfig:"PAPERLESS_TOKEN"`
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

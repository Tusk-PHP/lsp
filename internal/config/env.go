package config

import (
	"net/url"
	"os"
	"strings"
)

// DatabaseConfig holds parsed database connection parameters.
type DatabaseConfig struct {
	Driver   string // mysql, pgsql, sqlite
	Host     string
	Port     string
	Database string
	Username string
	Password string
}

// ParseEnvFile reads a .env file and returns its values by key; a key
// assigned twice keeps its last value, as phpdotenv does.
func ParseEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	entries := ParseDotenv(string(data))
	env := make(map[string]string, len(entries))
	for _, entry := range entries {
		env[entry.Key] = entry.Value
	}
	return env, nil
}

// ParseLaravelDatabaseConfig extracts database config from .env files in a Laravel project.
func ParseLaravelDatabaseConfig(rootPath string) *DatabaseConfig {
	env := loadEnvFiles(rootPath, ".env", ".env.local")
	if len(env) == 0 {
		return nil
	}

	driver := env["DB_CONNECTION"]
	if driver == "" {
		return nil
	}

	return &DatabaseConfig{
		Driver:   normalizeDatabaseDriver(driver),
		Host:     envOrDefault(env, "DB_HOST", "127.0.0.1"),
		Port:     envOrDefault(env, "DB_PORT", defaultPort(driver)),
		Database: env["DB_DATABASE"],
		Username: env["DB_USERNAME"],
		Password: env["DB_PASSWORD"],
	}
}

// ParseSymfonyDatabaseConfig extracts database config from Symfony .env files.
func ParseSymfonyDatabaseConfig(rootPath string) *DatabaseConfig {
	env := loadEnvFiles(rootPath, ".env", ".env.local")
	if len(env) == 0 {
		return nil
	}

	dbURL := env["DATABASE_URL"]
	if dbURL == "" {
		return nil
	}

	return parseDatabaseURL(dbURL)
}

// parseDatabaseURL parses a DATABASE_URL like mysql://user:pass@host:port/dbname
func parseDatabaseURL(rawURL string) *DatabaseConfig {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}

	cfg := &DatabaseConfig{
		Driver:   normalizeDatabaseDriver(u.Scheme),
		Host:     u.Hostname(),
		Port:     u.Port(),
		Database: strings.TrimPrefix(u.Path, "/"),
	}

	if u.User != nil {
		cfg.Username = u.User.Username()
		cfg.Password, _ = u.User.Password()
	}

	if cfg.Port == "" {
		cfg.Port = defaultPort(cfg.Driver)
	}

	return cfg
}

func loadEnvFiles(rootPath string, filenames ...string) map[string]string {
	merged := make(map[string]string)
	for _, name := range filenames {
		path := rootPath + "/" + name
		if env, err := ParseEnvFile(path); err == nil {
			for k, v := range env {
				merged[k] = v
			}
		}
	}
	return merged
}

func envOrDefault(env map[string]string, key, def string) string {
	if v, ok := env[key]; ok && v != "" {
		return v
	}
	return def
}

func normalizeDatabaseDriver(driver string) string {
	switch strings.ToLower(driver) {
	case "mysql", "mariadb":
		return "mysql"
	case "pgsql", "postgres", "postgresql":
		return "pgsql"
	case "sqlite", "sqlite3":
		return "sqlite"
	default:
		return driver
	}
}

func defaultPort(driver string) string {
	switch normalizeDatabaseDriver(driver) {
	case "mysql":
		return "3306"
	case "pgsql":
		return "5432"
	default:
		return ""
	}
}

package database

import (
	"strings"
	"testing"
	"time"
)

func TestMySQLConfigValidate(t *testing.T) {
	valid := testMySQLConfig()
	tests := []struct {
		name        string
		change      func(*MySQLConfig)
		errorPhrase string
	}{
		{
			name:        "missing host",
			change:      func(config *MySQLConfig) { config.Host = "" },
			errorPhrase: "host is required",
		},
		{
			name:        "invalid port",
			change:      func(config *MySQLConfig) { config.Port = 0 },
			errorPhrase: "port must be between",
		},
		{
			name:        "missing user",
			change:      func(config *MySQLConfig) { config.User = "" },
			errorPhrase: "user is required",
		},
		{
			name:        "missing password",
			change:      func(config *MySQLConfig) { config.Password = "" },
			errorPhrase: "password is required",
		},
		{
			name:        "missing database",
			change:      func(config *MySQLConfig) { config.Database = "" },
			errorPhrase: "database is required",
		},
		{
			name:        "idle connections exceed open connections",
			change:      func(config *MySQLConfig) { config.MaxIdleConns = config.MaxOpenConns + 1 },
			errorPhrase: "cannot exceed",
		},
		{
			name:        "missing ping timeout",
			change:      func(config *MySQLConfig) { config.PingTimeout = 0 },
			errorPhrase: "ping timeout must be positive",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.change(&config)
			err := config.Validate()
			if err == nil || !strings.Contains(err.Error(), test.errorPhrase) {
				t.Fatalf("expected error containing %q, got %v", test.errorPhrase, err)
			}
		})
	}
}

func TestMySQLConfigValidateAcceptsValidConfig(t *testing.T) {
	if err := testMySQLConfig().Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func testMySQLConfig() MySQLConfig {
	return MySQLConfig{
		Host:            "127.0.0.1",
		Port:            3306,
		User:            "taskpulse",
		Password:        "taskpulse_dev",
		Database:        "taskpulse",
		MaxOpenConns:    20,
		MaxIdleConns:    10,
		ConnMaxLifetime: 3 * time.Minute,
		ConnMaxIdleTime: time.Minute,
		ConnectTimeout:  5 * time.Second,
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    5 * time.Second,
		PingTimeout:     5 * time.Second,
	}
}

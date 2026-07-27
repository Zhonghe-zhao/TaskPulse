package database

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

func MySQLConfigFromEnv() (MySQLConfig, error) {
	config := MySQLConfig{
		Host:            envOrDefault("MYSQL_HOST", "127.0.0.1"),
		Port:            3306,
		User:            os.Getenv("MYSQL_USER"),
		Password:        os.Getenv("MYSQL_PASSWORD"),
		Database:        os.Getenv("MYSQL_DATABASE"),
		MaxOpenConns:    20,
		MaxIdleConns:    10,
		ConnMaxLifetime: 3 * time.Minute,
		ConnMaxIdleTime: time.Minute,
		ConnectTimeout:  5 * time.Second,
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    10 * time.Second,
		PingTimeout:     5 * time.Second,
	}

	integerSettings := []struct {
		name        string
		destination *int
	}{
		{name: "MYSQL_PORT", destination: &config.Port},
		{name: "MYSQL_MAX_OPEN_CONNS", destination: &config.MaxOpenConns},
		{name: "MYSQL_MAX_IDLE_CONNS", destination: &config.MaxIdleConns},
	}
	for _, setting := range integerSettings {
		if err := applyEnvironmentInt(setting.name, setting.destination); err != nil {
			return MySQLConfig{}, err
		}
	}

	durationSettings := []struct {
		name        string
		destination *time.Duration
	}{
		{name: "MYSQL_CONN_MAX_LIFETIME", destination: &config.ConnMaxLifetime},
		{name: "MYSQL_CONN_MAX_IDLE_TIME", destination: &config.ConnMaxIdleTime},
		{name: "MYSQL_CONNECT_TIMEOUT", destination: &config.ConnectTimeout},
		{name: "MYSQL_READ_TIMEOUT", destination: &config.ReadTimeout},
		{name: "MYSQL_WRITE_TIMEOUT", destination: &config.WriteTimeout},
		{name: "MYSQL_PING_TIMEOUT", destination: &config.PingTimeout},
	}
	for _, setting := range durationSettings {
		if err := applyEnvironmentDuration(setting.name, setting.destination); err != nil {
			return MySQLConfig{}, err
		}
	}

	if err := config.Validate(); err != nil {
		return MySQLConfig{}, fmt.Errorf("validate mysql environment config: %w", err)
	}
	return config, nil
}

func envOrDefault(name, fallback string) string {
	if value, exists := os.LookupEnv(name); exists {
		return value
	}
	return fallback
}

func applyEnvironmentInt(name string, destination *int) error {
	value, exists := os.LookupEnv(name)
	if !exists {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("parse %s as integer: %w", name, err)
	}
	*destination = parsed
	return nil
}

func applyEnvironmentDuration(name string, destination *time.Duration) error {
	value, exists := os.LookupEnv(name)
	if !exists {
		return nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("parse %s as duration: %w", name, err)
	}
	*destination = parsed
	return nil
}

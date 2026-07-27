package database

import (
	"strings"
	"testing"
	"time"
)

func TestMySQLConfigFromEnv(t *testing.T) {
	setRequiredMySQLEnvironment(t)
	t.Setenv("MYSQL_HOST", "mysql.internal")
	t.Setenv("MYSQL_PORT", "3307")
	t.Setenv("MYSQL_MAX_OPEN_CONNS", "30")
	t.Setenv("MYSQL_MAX_IDLE_CONNS", "12")
	t.Setenv("MYSQL_CONN_MAX_LIFETIME", "4m")
	t.Setenv("MYSQL_PING_TIMEOUT", "7s")

	config, err := MySQLConfigFromEnv()
	if err != nil {
		t.Fatalf("MySQLConfigFromEnv returned error: %v", err)
	}
	if config.Host != "mysql.internal" || config.Port != 3307 {
		t.Fatalf("unexpected address: %s:%d", config.Host, config.Port)
	}
	if config.MaxOpenConns != 30 || config.MaxIdleConns != 12 {
		t.Fatalf("unexpected pool settings: open=%d idle=%d", config.MaxOpenConns, config.MaxIdleConns)
	}
	if config.ConnMaxLifetime != 4*time.Minute || config.PingTimeout != 7*time.Second {
		t.Fatalf("unexpected duration settings: lifetime=%s ping=%s", config.ConnMaxLifetime, config.PingTimeout)
	}
}

func TestMySQLConfigFromEnvRejectsInvalidInteger(t *testing.T) {
	setRequiredMySQLEnvironment(t)
	t.Setenv("MYSQL_PORT", "not-a-number")

	_, err := MySQLConfigFromEnv()
	if err == nil || !strings.Contains(err.Error(), "MYSQL_PORT") {
		t.Fatalf("expected MYSQL_PORT parsing error, got %v", err)
	}
}

func TestMySQLConfigFromEnvRejectsInvalidDuration(t *testing.T) {
	setRequiredMySQLEnvironment(t)
	t.Setenv("MYSQL_PING_TIMEOUT", "soon")

	_, err := MySQLConfigFromEnv()
	if err == nil || !strings.Contains(err.Error(), "MYSQL_PING_TIMEOUT") {
		t.Fatalf("expected MYSQL_PING_TIMEOUT parsing error, got %v", err)
	}
}

func TestMySQLConfigFromEnvRequiresCredentials(t *testing.T) {
	setRequiredMySQLEnvironment(t)
	t.Setenv("MYSQL_USER", "")

	_, err := MySQLConfigFromEnv()
	if err == nil || !strings.Contains(err.Error(), "user is required") {
		t.Fatalf("expected missing user error, got %v", err)
	}
}

func setRequiredMySQLEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("MYSQL_HOST", "127.0.0.1")
	t.Setenv("MYSQL_PORT", "3306")
	t.Setenv("MYSQL_USER", "taskpulse")
	t.Setenv("MYSQL_PASSWORD", "taskpulse_dev")
	t.Setenv("MYSQL_DATABASE", "taskpulse")
	t.Setenv("MYSQL_MAX_OPEN_CONNS", "20")
	t.Setenv("MYSQL_MAX_IDLE_CONNS", "10")
	t.Setenv("MYSQL_CONN_MAX_LIFETIME", "3m")
	t.Setenv("MYSQL_CONN_MAX_IDLE_TIME", "1m")
	t.Setenv("MYSQL_CONNECT_TIMEOUT", "5s")
	t.Setenv("MYSQL_READ_TIMEOUT", "10s")
	t.Setenv("MYSQL_WRITE_TIMEOUT", "10s")
	t.Setenv("MYSQL_PING_TIMEOUT", "5s")
}

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

type MySQLConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	ConnectTimeout  time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	PingTimeout     time.Duration
}

func (c MySQLConfig) Validate() error {
	switch {
	case c.Host == "":
		return errors.New("mysql host is required")
	case c.Port < 1 || c.Port > 65535:
		return errors.New("mysql port must be between 1 and 65535")
	case c.User == "":
		return errors.New("mysql user is required")
	case c.Password == "":
		return errors.New("mysql password is required")
	case c.Database == "":
		return errors.New("mysql database is required")
	case c.MaxOpenConns <= 0:
		return errors.New("mysql max open connections must be positive")
	case c.MaxIdleConns < 0:
		return errors.New("mysql max idle connections cannot be negative")
	case c.MaxIdleConns > c.MaxOpenConns:
		return errors.New("mysql max idle connections cannot exceed max open connections")
	case c.ConnMaxLifetime <= 0:
		return errors.New("mysql connection max lifetime must be positive")
	case c.ConnMaxIdleTime <= 0:
		return errors.New("mysql connection max idle time must be positive")
	case c.ConnectTimeout <= 0:
		return errors.New("mysql connect timeout must be positive")
	case c.ReadTimeout <= 0:
		return errors.New("mysql read timeout must be positive")
	case c.WriteTimeout <= 0:
		return errors.New("mysql write timeout must be positive")
	case c.PingTimeout <= 0:
		return errors.New("mysql ping timeout must be positive")
	default:
		return nil
	}
}

func OpenMySQL(ctx context.Context, config MySQLConfig) (*sql.DB, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate mysql config: %w", err)
	}

	driverConfig := mysqldriver.NewConfig()
	driverConfig.User = config.User
	driverConfig.Passwd = config.Password
	driverConfig.Net = "tcp"
	driverConfig.Addr = net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	driverConfig.DBName = config.Database
	driverConfig.ParseTime = true
	driverConfig.Loc = time.UTC
	driverConfig.Timeout = config.ConnectTimeout
	driverConfig.ReadTimeout = config.ReadTimeout
	driverConfig.WriteTimeout = config.WriteTimeout
	driverConfig.Collation = "utf8mb4_0900_ai_ci"

	connector, err := mysqldriver.NewConnector(driverConfig)
	if err != nil {
		return nil, fmt.Errorf("create mysql connector: %w", err)
	}

	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	pingContext, cancel := context.WithTimeout(ctx, config.PingTimeout)
	defer cancel()
	if err := db.PingContext(pingContext); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	return db, nil
}

package testfixtures

import (
	"database/sql"
	"fmt"
	"math/rand"
	"os"
)

type DBConfig struct {
	Username string
	Password string
	Hostname string
	Port     uint16
}

func (d DBConfig) String() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/", d.Username, d.Password, d.Hostname, d.Port)
}

var DefaultDBConfig = DBConfig{
	Username: "root",
	Password: "",
	Hostname: "localhost",
	Port:     3307,
}

func init() {
	if pw := os.Getenv("DB_PASSWORD"); pw != "" {
		DefaultDBConfig.Password = pw
	}
}

type TestDatabase struct {
	DBName    string
	ControlDB *sql.DB
	*sql.DB
}

func NewTestDatabase() (*TestDatabase, error) {
	controlDb, err := sql.Open("mysql", DefaultDBConfig.String())
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("thunder_test_%d", rand.Intn(1<<30))
	_, err = controlDb.Exec(fmt.Sprintf("CREATE DATABASE %s", name))
	if err != nil {
		controlDb.Close()
		return nil, err
	}

	db, err := sql.Open("mysql", DefaultDBConfig.String()+name)
	if err != nil {
		controlDb.Close()
		return nil, err
	}

	return &TestDatabase{
		DB:        db,
		DBName:    name,
		ControlDB: controlDb,
	}, nil
}

func (t *TestDatabase) Close() error {
	if err := t.DB.Close(); err != nil {
		return err
	}
	if _, err := t.ControlDB.Exec(fmt.Sprintf("DROP DATABASE %s", t.DBName)); err != nil {
		return err
	}
	if err := t.ControlDB.Close(); err != nil {
		return err
	}
	return nil
}

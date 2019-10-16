package testfixtures

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
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
	Password: "dev",
	Hostname: "localhost",
	Port:     3307,
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

	var bytes [4]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("thunder_test_%s", hex.EncodeToString(bytes[:]))
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

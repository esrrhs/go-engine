package tmap

import (
	"database/sql"
	"github.com/esrrhs/go-engine/src/loggo"
	"strconv"
)

type TMysql struct {
	gdb   *sql.DB
	dsn   string
	table string
	day   int
}

func NewTMysql(dsn string, table string, day int) *TMysql {
	return &TMysql{dsn: dsn, table: table, day: day}
}

func (t *TMysql) Load() error {

	loggo.Info("mysql dht Load start")

	db, err := sql.Open("mysql", t.dsn)
	if err != nil {
		loggo.Error("TMysql Open fail %v", err)
		return err
	}
	t.gdb = db

	loggo.Info("mysql dht Load ok")

	_, err = t.gdb.Exec("CREATE DATABASE IF NOT EXISTS tmysql")
	if err != nil {
		loggo.Error("TMysql CREATE DATABASE fail %v", err)
		return err
	}

	_, err = t.gdb.Exec("CREATE TABLE IF NOT EXISTS tmysql." + t.table + "(" +
		"name VARCHAR(40) NOT NULL," +
		"value VARCHAR(1000) NOT NULL," +
		"time DATETIME NOT NULL," +
		"PRIMARY KEY(name));")
	if err != nil {
		loggo.Error("TMysql CREATE TABLE fail %v", err)
		return err
	}

	num := t.GetSize()
	loggo.Info("TMysql size %v", num)

	return nil
}

func (t *TMysql) Insert(key string, value string) error {

	tx, err := t.gdb.Begin()
	if err != nil {
		loggo.Error("TMysql Begin fail %v", err)
		return err
	}
	stmt, err := tx.Prepare("insert IGNORE into tmysql." + t.table + "(name, value, time) values(?, ?, NOW())")
	if err != nil {
		loggo.Error("TMysql Prepare fail %v", err)
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(key, value)
	if err != nil {
		loggo.Error("TMysql insert fail %v", err)
		return err
	}
	err = tx.Commit()
	if err != nil {
		loggo.Error("TMysql Commit fail %v", err)
		return err
	}

	t.gdb.Exec("delete from tmysql." + t.table + " where (TO_DAYS(NOW()) - TO_DAYS(time)) >= " + strconv.Itoa(t.day))

	num := t.GetSize()

	loggo.Info("TMysql InsertSpider ok %v %v %v", key, value, num)

	return nil
}

func (t *TMysql) GetSize() int {

	rows, err := t.gdb.Query("select count(*) from tmysql." + t.table)
	if err != nil {
		loggo.Error("TMysql Query fail %v", err)
		return 0
	}
	defer rows.Close()

	rows.Next()

	var num int
	err = rows.Scan(&num)
	if err != nil {
		loggo.Error("TMysql Scan fail %v", err)
		return 0
	}

	return num
}

func (t *TMysql) Has(key string) bool {

	rows, err := t.gdb.Query("select name, value from tmysql." + t.table + " where name='" + key + "'")
	if err != nil {
		loggo.Error("TMysql Query fail %v", err)
		return false
	}
	defer rows.Close()

	for rows.Next() {
		return true
	}

	return false
}

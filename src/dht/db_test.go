package dht

import (
	"github.com/esrrhs/go-engine/src/loggo"
	"github.com/go-sql-driver/mysql"
	"testing"
)

func Test0001(t *testing.T) {

	dbconfig := mysql.NewConfig()
	dbconfig.User = "root"
	dbconfig.Passwd = "123123"
	dbconfig.Addr = "192.168.0.106:4406"
	dbconfig.Net = "tcp"

	f := Load(dbconfig.FormatDSN(), 10)
	if f == nil {
		return
	}

	InsertSpider("aaa", "test")
	InsertSpider("bbb", "test")
	InsertSpider("ccc", "test")
	loggo.Info("Last %v", Last(2))
	loggo.Info("Find %v", Find("t", 2))
}

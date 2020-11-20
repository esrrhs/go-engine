package dht

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"github.com/esrrhs/go-engine/src/common"
	"github.com/esrrhs/go-engine/src/loggo"
	"github.com/shiyanhui/dht"
	"strconv"
)

var gdb *sql.DB
var gcb func(infohash string, name string)

func Load(dsn string) error {

	loggo.Info("mysql dht Load start")

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		panic(err)
	}
	gdb = db

	loggo.Info("mysql dht Load ok")

	_, err = gdb.Exec("CREATE DATABASE IF NOT EXISTS dht")
	if err != nil {
		loggo.Error("CREATE dht DATABASE fail %v", err)
		return nil
	}

	_, err = gdb.Exec("CREATE TABLE  IF NOT EXISTS dht.meta_info(" +
		"infohash VARCHAR(40) NOT NULL," +
		"name VARCHAR(400) NOT NULL," +
		"time DATETIME NOT NULL," +
		"PRIMARY KEY(infohash));")
	if err != nil {
		loggo.Error("CREATE dht TABLE fail %v", err)
		return nil
	}

	num := GetSize()
	loggo.Info("mysql dht dht size %v", num)

	go Crawl()

	return nil
}

func SetCallback(cb func(infohash string, name string)) {
	gcb = cb
}

type file struct {
	Path   []interface{} `json:"path"`
	Length int           `json:"length"`
}

type bitTorrent struct {
	InfoHash string `json:"infohash"`
	Name     string `json:"name"`
	Files    []file `json:"files,omitempty"`
	Length   int    `json:"length,omitempty"`
}

func OnCrawl(w *dht.Wire) {
	defer common.CrashLog()

	for resp := range w.Response() {
		loggo.Info("OnCrawl resp bytes %v", len(resp.MetadataInfo))

		metadata, err := dht.Decode(resp.MetadataInfo)
		if err != nil {
			continue
		}
		info := metadata.(map[string]interface{})

		if _, ok := info["name"]; !ok {
			continue
		}

		bt := bitTorrent{
			InfoHash: hex.EncodeToString(resp.InfoHash),
			Name:     info["name"].(string),
		}

		if v, ok := info["files"]; ok {
			files := v.([]interface{})
			bt.Files = make([]file, len(files))

			for i, item := range files {
				f := item.(map[string]interface{})
				bt.Files[i] = file{
					Path:   f["path"].([]interface{}),
					Length: f["length"].(int),
				}
			}
		} else if _, ok := info["length"]; ok {
			bt.Length = info["length"].(int)
		}

		data, err := json.Marshal(bt)
		if err == nil {
			loggo.Info("Crawl %s", data)

			InsertSpider(bt.InfoHash, bt.Name)
		}
	}
}

func InsertSpider(infohash string, name string) {

	tx, err := gdb.Begin()
	if err != nil {
		loggo.Error("Begin sqlite3 fail %v", err)
		return
	}
	stmt, err := tx.Prepare("insert IGNORE into dht.meta_info(infohash, name, time) values(?, ?, NOW())")
	if err != nil {
		loggo.Error("Prepare sqlite3 fail %v", err)
		return
	}
	defer stmt.Close()
	_, err = stmt.Exec(infohash, name)
	if err != nil {
		loggo.Error("insert sqlite3 fail %v", err)
	}
	tx.Commit()

	gdb.Exec("delete from dht.meta_info where (TO_DAYS(NOW()) - TO_DAYS(time)) >= 30")

	num := GetSize()

	if gcb != nil {
		gcb(infohash, name)
	}

	loggo.Info("InsertSpider size %v %v %v", infohash, name, num)
}

func GetSize() int {

	rows, err := gdb.Query("select count(*) from dht.meta_info")
	if err != nil {
		loggo.Error("Query sqlite3 fail %v", err)
		return 0
	}
	defer rows.Close()

	rows.Next()

	var num int
	err = rows.Scan(&num)
	if err != nil {
		loggo.Error("Scan sqlite3 fail %v", err)
		return 0
	}

	return num
}

func Crawl() {
	defer common.CrashLog()

	w := dht.NewWire(65536, 1024, 256)
	go OnCrawl(w)
	go func() {
		defer common.CrashLog()
		w.Run()
	}()

	config := dht.NewCrawlConfig()
	config.OnAnnouncePeer = func(infoHash, ip string, port int) {
		w.Request([]byte(infoHash), ip, port)
	}
	d := dht.New(config)

	go func() {
		defer common.CrashLog()
		d.Run()
	}()
}

type FindData struct {
	Infohash string
	Name     string
}

func Last(n int) []FindData {
	var ret []FindData

	retmap := make(map[string]string)

	rows, err := gdb.Query("select infohash,name from dht.meta_info order by time desc limit 0," + strconv.Itoa(n))
	if err != nil {
		loggo.Error("Query sqlite3 fail %v", err)
	}
	defer rows.Close()

	for rows.Next() {

		var infohash string
		var name string
		err = rows.Scan(&infohash, &name)
		if err != nil {
			loggo.Error("Scan sqlite3 fail %v", err)
		}

		_, ok := retmap[infohash]
		if ok {
			continue
		}
		retmap[infohash] = name

		ret = append(ret, FindData{infohash, name})
	}

	return ret
}

func Find(str string, max int) []FindData {
	var ret []FindData

	rows, err := gdb.Query("select infohash,name from dht.meta_info where name like '%" + str + "%' limit 0," + strconv.Itoa(max))
	if err != nil {
		loggo.Error("Query sqlite3 fail %v", err)
	}
	defer rows.Close()

	for rows.Next() {

		var infohash string
		var name string
		err = rows.Scan(&infohash, &name)
		if err != nil {
			loggo.Error("Scan sqlite3 fail %v", err)
		}

		ret = append(ret, FindData{infohash, name})
	}

	return ret
}

package sql

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"

	"github.com/1024casts/snake/pkg/breaker"
	"github.com/1024casts/snake/pkg/conf"
	logger "github.com/1024casts/snake/pkg/log"
	xtime "github.com/1024casts/snake/pkg/time"
)

var (
	cfgFile = pflag.StringP("config", "c", "", "snake config file path.")
)

func TestMySQL(t *testing.T) {
	pflag.Parse()

	*cfgFile = "../../../config/config.yaml"

	cfg, err := conf.Init(*cfgFile)
	if err != nil {
		panic(err)
	}

	// init log
	logger.Init(&cfg.Logger)

	bc := &breaker.Config{
		Window:  xtime.Duration(10 * time.Second),
		Bucket:  10,
		Request: 100,
	}
	// dsn := os.Getenv("TEST_MYSQL_DSN")
	dsn := "root:12345678@tcp(127.0.0.1:3306)/test?timeout=5s&readTimeout=5s&writeTimeout=5s&parseTime=true&loc=Local&charset=utf8"
	if dsn == "" {
		t.Skipf("TEST_MYSQL_DSN is empty, sql test skipped")
	}
	dsn = dsn + "?timeout=5s&readTimeout=5s&writeTimeout=5s&parseTime=true&loc=Local&charset=utf8"
	c := &Config{
		DSN:             dsn,
		MaxOpenConn:     10,
		MaxIdleConn:     5,
		ConnMaxLifeTime: xtime.Duration(time.Minute),
		QueryTimeout:    xtime.Duration(time.Minute),
		ExecTimeout:     xtime.Duration(time.Minute),
		TranTimeout:     xtime.Duration(time.Minute),
		Breaker:         bc,
	}

	db := NewMySQL(c)
	defer db.Close()
	testPing(t, db)
	testTable(t, db)
	testExec(t, db)
	testQuery(t, db)
	testQueryRow(t, db)
	testPrepare(t, db)
	testPrepared(t, db)
	testTransaction(t, db)
	testMaster(t, db)
	testMasterPanic(t, db)
}

func testMaster(t *testing.T, db *DB) {
	master := db.Master()
	if len(master.read) != 0 {
		t.Errorf("expect master read conn is 0, get %d", len(master.read))
	}
	if master.write != db.write {
		t.Errorf("expect master write conn equal to origin db write conn")
	}
}

func testMasterPanic(t *testing.T, db *DB) {
	defer func() {
		err := recover()
		if err == nil || err != ErrNoMaster {
			t.Errorf("expect panic err=ErrNoMaster get %v", err)
		}
	}()
	db.Master().Master()
}

func testTransaction(t *testing.T, db *DB) {
	var (
		tx      *Tx
		err     error
		execSQL = "INSERT INTO test(name) VALUES(?)"
		selSQL  = "SELECT name FROM test WHERE name=?"
		txstmt  *Stmt
	)
	if tx, err = db.Begin(context.TODO()); err != nil {
		t.Errorf("MySQL: db transaction Begin err(%v)", err)
		tx.Rollback()
		return
	}
	t.Log("MySQL: db transaction begin")
	if txstmt, err = tx.Prepare(execSQL); err != nil {
		t.Errorf("MySQL: tx.Prepare err(%v)", err)
	}
	if stmt := tx.Stmt(txstmt); stmt == nil {
		t.Errorf("MySQL:tx.Stmt err(%v)", err)
	}
	// exec
	if _, err = tx.Exec(execSQL, "tx1"); err != nil {
		t.Errorf("MySQL: tx.Exec err(%v)", err)
		tx.Rollback()
		return
	}
	t.Logf("MySQL:tx.Exec tx1")
	if _, err = tx.Exec(execSQL, "tx1"); err != nil {
		t.Errorf("MySQL: tx.Exec err(%v)", err)
		tx.Rollback()
		return
	}
	t.Logf("MySQL:tx.Exec tx1")
	// query
	rows, err := tx.Query(selSQL, "tx2")
	if err != nil {
		t.Errorf("MySQL:tx.Query err(%v)", err)
		tx.Rollback()
		return
	}
	rows.Close()
	t.Log("MySQL: tx.Query tx2")
	// queryrow
	var name string
	row := tx.QueryRow(selSQL, "noexist")
	if err = row.Scan(&name); err != sql.ErrNoRows {
		t.Errorf("MySQL: queryRow name: noexist")
	}
	if err = tx.Commit(); err != nil {
		t.Errorf("MySQL:tx.Commit err(%v)", err)
	}
	if err = tx.Commit(); err != nil {
		t.Logf("MySQL:tx.Commit err(%v)", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Logf("MySQL:tx Rollback err(%v)", err)
	}
}

func testPing(t *testing.T, db *DB) {
	if err := db.Ping(context.TODO()); err != nil {
		t.Errorf("MySQL: ping error(%v)", err)
		t.FailNow()
	} else {
		t.Log("MySQL: ping ok")
	}
}

func testTable(t *testing.T, db *DB) {
	table := "CREATE TABLE IF NOT EXISTS `test` (`id` int(11) NOT NULL AUTO_INCREMENT COMMENT '自增ID', `name` varchar(16) NOT NULL DEFAULT '' COMMENT '名称', PRIMARY KEY (`id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8"
	if _, err := db.Exec(context.TODO(), table); err != nil {
		t.Errorf("MySQL: create table error(%v)", err)
	} else {
		t.Log("MySQL: create table ok")
	}
}

func testExec(t *testing.T, db *DB) {
	sql := "INSERT INTO test(name) VALUES(?)"
	if _, err := db.Exec(context.TODO(), sql, "test"); err != nil {
		t.Errorf("MySQL: insert error(%v)", err)
	} else {
		t.Log("MySQL: insert ok")
	}
}

func testQuery(t *testing.T, db *DB) {
	sql := "SELECT name FROM test WHERE name=?"
	rows, err := db.Query(context.TODO(), sql, "test")
	if err != nil {
		t.Errorf("MySQL: query error(%v)", err)
	}
	defer rows.Close()
	for rows.Next() {
		name := ""
		if err := rows.Scan(&name); err != nil {
			t.Errorf("MySQL: query scan error(%v)", err)
		} else {
			t.Logf("MySQL: query name: %s", name)
		}
	}
}

func testQueryRow(t *testing.T, db *DB) {
	sql := "SELECT name FROM test WHERE name=?"
	name := ""
	row := db.QueryRow(context.TODO(), sql, "test")
	if err := row.Scan(&name); err != nil {
		t.Errorf("MySQL: queryRow error(%v)", err)
	} else {
		t.Logf("MySQL: queryRow name: %s", name)
	}
}

func testPrepared(t *testing.T, db *DB) {
	sql := "SELECT name FROM test WHERE name=?"
	name := ""
	stmt := db.Prepared(sql)
	row := stmt.QueryRow(context.TODO(), "test")
	if err := row.Scan(&name); err != nil {
		t.Errorf("MySQL: prepared query error(%v)", err)
	} else {
		t.Logf("MySQL: prepared query name: %s", name)
	}
	if err := stmt.Close(); err != nil {
		t.Errorf("MySQL:stmt.Close err(%v)", err)
	}
}

func testPrepare(t *testing.T, db *DB) {
	var (
		selsql  = "SELECT name FROM test WHERE name=?"
		execsql = "INSERT INTO test(name) VALUES(?)"
		name    = ""
	)
	selstmt, err := db.Prepare(selsql)
	if err != nil {
		t.Errorf("MySQL:Prepare err(%v)", err)
		return
	}
	row := selstmt.QueryRow(context.TODO(), "noexit")
	if err = row.Scan(&name); err == sql.ErrNoRows {
		t.Logf("MySQL: prepare query error(%v)", err)
	} else {
		t.Errorf("MySQL: prepared query name: noexist")
	}
	rows, err := selstmt.Query(context.TODO(), "test")
	if err != nil {
		t.Errorf("MySQL:stmt.Query err(%v)", err)
	}
	rows.Close()
	execstmt, err := db.Prepare(execsql)
	if err != nil {
		t.Errorf("MySQL:Prepare err(%v)", err)
		return
	}
	if _, err := execstmt.Exec(context.TODO(), "test"); err != nil {
		t.Errorf("MySQL: stmt.Exec(%v)", err)
	}
}

func BenchmarkMySQL(b *testing.B) {
	c := &Config{
		DSN:             "root:12345678@tcp(127.0.0.1:3306)/test?timeout=5s&readTimeout=5s&writeTimeout=5s&parseTime=true&loc=Local&charset=utf8",
		MaxOpenConn:     10,
		MaxIdleConn:     5,
		ConnMaxLifeTime: xtime.Duration(time.Minute),
	}
	db := NewMySQL(c)
	defer db.Close()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sql := "SELECT name FROM test WHERE name=?"
			rows, err := db.Query(context.TODO(), sql, "test")
			if err == nil {
				for rows.Next() {
					var name string
					if err = rows.Scan(&name); err != nil {
						break
					}
				}
				rows.Close()
			}
		}
	})
}

func TestParseAddrDSN(t *testing.T) {
	t.Run("test parse addr dsn", func(t *testing.T) {
		addr := parseDSNAddr("test:test@tcp(127.0.0.1:3306)/test?timeout=5s&readTimeout=5s&writeTimeout=5s&parseTime=true&loc=Local&charset=utf8")
		assert.Equal(t, "172.16.0.148:3306", addr)
	})
	t.Run("test password has @", func(t *testing.T) {
		addr := parseDSNAddr("root:root@dev@tcp(1.2.3.4:3306)/abc?timeout=1s&readTimeout=1s&writeTimeout=1s&parseTime=true&loc=Local&charset=utf8mb4,utf8")
		assert.Equal(t, "1.2.3.4:3306", addr)
	})
}

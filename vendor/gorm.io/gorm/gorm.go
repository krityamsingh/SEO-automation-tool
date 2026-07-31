package gorm

import (
	"database/sql"
	"gorm.io/gorm/logger"
)

type Dialector interface {
	Name() string
}

type Option interface {
	Apply(*Config)
}

type Config struct {
	Logger logger.Interface
}

func (c *Config) Apply(cfg *Config) {
	if c.Logger != nil {
		cfg.Logger = c.Logger
	}
}

type Statement struct {
	Model interface{}
}

type DB struct {
	Error        error
	RowsAffected int64
	Statement    *Statement
	Config       *Config
}

func Open(dialector Dialector, opts ...Option) (*DB, error) {
	cfg := &Config{}
	for _, opt := range opts {
		if opt != nil {
			opt.Apply(cfg)
		}
	}
	db := &DB{Config: cfg}
	db.Statement = &Statement{Model: db}
	return db, nil
}

func Expr(expr string, args ...interface{}) interface{} {
	return expr
}

func (db *DB) AutoMigrate(dst ...interface{}) error {
	return nil
}

func (db *DB) Create(value interface{}) *DB {
	db.RowsAffected = 1
	return db
}

func (db *DB) Save(value interface{}) *DB {
	db.RowsAffected = 1
	return db
}

func (db *DB) First(dest interface{}, conds ...interface{}) *DB {
	return db
}

func (db *DB) FirstOrCreate(dest interface{}, conds ...interface{}) *DB {
	return db
}

func (db *DB) Find(dest interface{}, conds ...interface{}) *DB {
	return db
}

func (db *DB) Where(query interface{}, args ...interface{}) *DB {
	return db
}

func (db *DB) Order(value interface{}) *DB {
	return db
}

func (db *DB) Limit(limit int) *DB {
	return db
}

func (db *DB) Model(value interface{}) *DB {
	db.Statement.Model = value
	return db
}

func (db *DB) Update(column string, value interface{}) *DB {
	db.RowsAffected = 1
	return db
}

func (db *DB) UpdateColumn(column string, value interface{}) *DB {
	db.RowsAffected = 1
	return db
}

func (db *DB) Updates(values interface{}) *DB {
	db.RowsAffected = 1
	return db
}

func (db *DB) Delete(value interface{}, conds ...interface{}) *DB {
	db.RowsAffected = 1
	return db
}

func (db *DB) DB() (*sql.DB, error) {
	return &sql.DB{}, nil
}

func (db *DB) Transaction(fc func(tx *DB) error, opts ...*sql.TxOptions) (err error) {
	return fc(db)
}

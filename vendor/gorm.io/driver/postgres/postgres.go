package postgres

import (
	"gorm.io/gorm"
)

type Dialector struct {
	DSN string
}

func (d Dialector) Name() string {
	return "postgres"
}

func Open(dsn string) gorm.Dialector {
	return Dialector{DSN: dsn}
}

package sqlite

import (
	"gorm.io/gorm"
)

type Dialector struct {
	Path string
}

func (d Dialector) Name() string {
	return "sqlite"
}

func Open(path string) gorm.Dialector {
	return Dialector{Path: path}
}

package app

import "gorm.io/gorm"

type Dependencies struct {
	DB *gorm.DB
}

var deps Dependencies

func SetDependencies(d Dependencies) {
	deps = d
}

func Get() Dependencies {
	return deps
}

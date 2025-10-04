package app

import (
	"gorm.io/gorm"

	"github.com/telnet2/mysql-vfs/services/metadata/internal/policy"
)

type Dependencies struct {
	DB              *gorm.DB
	PolicyRegistry  *policy.Registry
	PolicyEvaluator *policy.Evaluator
	PolicyValidator *policy.Validator
	PolicyTriggerEngine *policy.TriggerEngine
}

var deps Dependencies

func SetDependencies(d Dependencies) {
	deps = d
}

func Get() Dependencies {
	return deps
}

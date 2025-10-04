package service

import "errors"

var (
	ErrDirectoryNotFound = errors.New("directory_not_found")
	ErrFileNotFound      = errors.New("file_not_found")
	ErrNameConflict      = errors.New("name_conflict")
	ErrInvalidRequest    = errors.New("invalid_request")
	ErrVersionConflict   = errors.New("version_conflict")
	ErrPolicyForbidden   = errors.New("policy_forbidden")
	ErrSchemaValidation  = errors.New("schema_validation_failed")
)

package catalog

import "errors"

var (
	ErrNotFound      = errors.New("catalog item not found")
	ErrAlreadyExists = errors.New("bot with this username is already published")
	ErrInvalidInput  = errors.New("invalid submission")
	ErrNotPending    = errors.New("submission is not pending")
	ErrUnauthorized  = errors.New("not authorized")
)

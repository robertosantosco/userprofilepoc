package comparer

import (
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func IgnoreFieldsFor[T any](fields ...string) cmp.Option {
	var t T
	return cmpopts.IgnoreFields(t, fields...)
}
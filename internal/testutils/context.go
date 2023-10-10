package testutils

import (
	"context"
	"testing"

	"go.6river.tech/gosix/testutils"
)

func ContextForTest(t testing.TB) context.Context {
	return testutils.ContextForTest(t)
}

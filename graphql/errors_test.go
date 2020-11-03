package graphql_test

import (
	"errors"
	"testing"

	"github.com/northvolt/thunder/graphql"
	"github.com/stretchr/testify/assert"
)

type Wrapper interface {
	Unwrap() error
}

func TestNewSafeError(t *testing.T) {
	err := graphql.NewSafeError("This is an error.")
	wrapperError, ok := err.(Wrapper)
	assert.True(t, ok)
	assert.Nil(t, wrapperError.Unwrap())
}

func TestWrapAsSafeError(t *testing.T) {
	sourceErr := errors.New("I am the source error.")
	err := graphql.WrapAsSafeError(sourceErr, "This is an error.")
	wrapperError, ok := err.(Wrapper)
	assert.True(t, ok)
	assert.Equal(t, sourceErr, wrapperError.Unwrap())
}

package validators

import (
	"errors"
	"strings"
)

var ErrInvalid = errors.New("invalid household")

func Name(v string) error {
	n := len(strings.TrimSpace(v))
	if n < 1 || n > 120 {
		return ErrInvalid
	}
	return nil
}

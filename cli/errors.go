package cli

import (
	"errors"

	"github.com/tamnd/cstheoryse-cli/cstheoryse"
)

func isNotFound(err error) bool {
	return errors.Is(err, cstheoryse.ErrNotFound)
}

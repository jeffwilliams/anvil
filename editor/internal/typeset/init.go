package typeset

import (
	"fmt"

	"github.com/timtadh/data-structures/errors"
)

func init() {
	installFasterErrorFormattersInTimtadhDataStructures()
}

var simpleError = fmt.Errorf("operation failed")

func simpleErrorFmter(a ...interface{}) error {
	return simpleError
}

// installFasterErrorFormattersInTimtadhDataStructures changes the error formatters
// used by the github.com/timtadh/data-structures packages so that they are faster.
// The default ones generate a stacktrace each time a hash entry is not found or removed
func installFasterErrorFormattersInTimtadhDataStructures() {
	for k := range errors.Errors {
		errors.Errors[k] = simpleErrorFmter
	}

}

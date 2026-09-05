package testkit

import (
	"math/rand/v2"
	"strconv"
)

// ResourceID returns a resource ID under the prefix that no other spec
// carries, so the specs share one database without sharing a resource.
func ResourceID(prefix string) string {
	return prefix + "-" + strconv.FormatUint(rand.Uint64(), 36)
}

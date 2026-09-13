package config

import (
	"cmp"
	"strings"

	"github.com/routatic/proxy/internal/site"
)

// CompareProviderDisplay orders labels only; routing, fallback and key order
// remain owned by their configured chains. The rank itself comes from the
// platform registry so this order cannot drift from the set of known platforms.
func CompareProviderDisplay(a, b string) int {
	return cmp.Or(cmp.Compare(site.Order(a), site.Order(b)), strings.Compare(a, b))
}

package filter

import "github.com/asergeyev/nradix"

// NewCIDR returns a new CIDR filter.
func NewCIDR(filtered []string) *CIDRFilter {
	tree := nradix.NewTree(len(filtered))
	f := &CIDRFilter{t: tree}
	for _, cidr := range filtered {
		f.t.AddCIDR(cidr, false)
	}
	return f
}

// CIDRFilter is a radix tree that holds filtered ips.
type CIDRFilter struct {
	t *nradix.Tree
}

// Allowed tells us if the input address/cidr is allowed.
func (f *CIDRFilter) Allowed(cidr string) (bool, error) {
	allowed, err := f.t.FindCIDR(cidr)
	if err != nil {
		return false, err
	}
	// If there's no entry the return value is nil. So it it not filtered.
	if allowed == nil {
		return true, nil
	}
	// Otherwise it is.
	// TODO(ro) 2017-10-04 Should we just return false instead of asserting?
	return allowed.(bool), nil
}

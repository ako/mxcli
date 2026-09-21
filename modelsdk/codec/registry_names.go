// SPDX-License-Identifier: Apache-2.0

package codec

import "sort"

// TypeNames returns every $Type string the registry can decode, sorted.
//
// Aliases are included: the registry stores a storage name and its qualified
// twin as two forward entries, and a caller matching a stored `$Type` needs
// both — the whole point of the storage-name split is that a document carries
// the one the SDK docs do not name.
func (r *TypeRegistry) TypeNames() []string {
	r.mu.RLock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	r.mu.RUnlock()
	sort.Strings(names)
	return names
}

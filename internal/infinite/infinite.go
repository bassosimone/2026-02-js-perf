// SPDX-License-Identifier: AGPL-3.0-or-later

package infinite

import "io"

// Reader is an infinite [io.Reader].
type Reader struct{}

var _ io.Reader = Reader{}

// Read implements [io.Reader].
func (r Reader) Read(data []byte) (int, error) {
	clear(data)
	return len(data), nil
}

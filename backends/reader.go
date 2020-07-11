package backends

import "io"

type SeekPartReader interface {
	io.Reader
	SeekPart(part int) error
}

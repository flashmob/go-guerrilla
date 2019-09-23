package chunk

import (
	"errors"
	"fmt"
	"io"
)

type chunkedReader struct {
	db    Storage
	email *Email
	// part requests a part. If 0, all the parts are read sequentially
	part int
	i, j int

	cache cachedChunks
}

// NewChunkedReader loads the email and selects which mime-part Read will read, starting from 1
// if part is 0, Read will read in the entire message. 1 selects the first part, 2 2nd, and so on..
func NewChunkedReader(db Storage, email *Email, part int) (*chunkedReader, error) {
	r := new(chunkedReader)
	r.db = db
	if email == nil {
		return nil, errors.New("nil email")
	} else {
		r.email = email
	}
	if err := r.SeekPart(part); err != nil {
		return nil, err
	}
	r.cache = cachedChunks{
		db: db,
	}
	return r, nil
}

// SeekPart resets the reader. The part argument chooses which part Read will read in
// If part is 1, it will return the first part
// If part is 0, Read will return the entire message
func (r *chunkedReader) SeekPart(part int) error {
	if parts := len(r.email.partsInfo.Parts); parts == 0 {
		return errors.New("email has mime parts missing")
	} else if part > parts {
		return errors.New("no such part available")
	}
	r.part = part
	if part > 0 {
		r.i = part - 1
	}
	r.j = 0
	return nil
}

type cachedChunks struct {
	chunks    []*Chunk
	hashIndex map[int]HashKey
	db        Storage
}

const chunkCachePreload = 2

// warm allocates the chunk cache, and gets the first few and stores them in the cache
func (c *cachedChunks) warm(hashes ...HashKey) (int, error) {

	if c.hashIndex == nil {
		c.hashIndex = make(map[int]HashKey, len(hashes))
	}
	if c.chunks == nil {
		c.chunks = make([]*Chunk, 0, 100)
	}
	if len(c.chunks) > 0 {
		// already been filled
		return len(c.chunks), nil
	}
	// let's pre-load some hashes.
	preload := chunkCachePreload
	if len(hashes) < preload {
		preload = len(hashes)
	}
	if chunks, err := c.db.GetChunks(hashes[0:preload]...); err != nil {
		return 0, err
	} else {
		for i := range hashes {
			c.hashIndex[i] = hashes[i]
			if i < preload {
				c.chunks = append(c.chunks, chunks[i])
			} else {
				// don't pre-load
				c.chunks = append(c.chunks, nil) // nil will be a placeholder for our chunk
			}
		}
	}
	return len(c.chunks), nil
}

// get returns a chunk. If the chunk doesn't exist, it gets it and pre-loads the next few
// also removes the previous chunks that now have become stale
func (c *cachedChunks) get(i int) (*Chunk, error) {
	if i > len(c.chunks) {
		return nil, errors.New("not enough chunks")
	}
	if c.chunks[i] != nil {
		// cache hit!
		return c.chunks[i], nil
	} else {
		var toGet []HashKey
		if key, ok := c.hashIndex[i]; ok {
			toGet = append(toGet, key)
		} else {
			return nil, errors.New(fmt.Sprintf("hash for key [%s] not found", key))
		}
		// make a list of chunks to load (extra ones to be pre-loaded)
		for to := i + 1; to < len(c.chunks) || to > chunkCachePreload+i; to++ {
			if key, ok := c.hashIndex[to]; ok {
				toGet = append(toGet, key)
			}
		}
		if chunks, err := c.db.GetChunks(toGet...); err != nil {
			return nil, err
		} else {
			// cache the pre-loaded chunks
			for j := i; j < len(c.chunks); j++ {
				c.chunks[j] = chunks[j-i]
				c.hashIndex[j] = toGet[j-i]
			}
			// remove any old ones (walk back)
			if i-1 > -1 {
				for j := i - 1; j > -1; j-- {
					if c.chunks[j] != nil {
						c.chunks[j] = nil
					} else {
						break
					}
				}
			}

			// return the chunk asked for
			return chunks[0], nil
		}
	}
}

func (c *cachedChunks) empty() {
	for i := range c.chunks {
		c.chunks[i] = nil
	}
	c.chunks = c.chunks[:] // set len to 0
	for key := range c.hashIndex {
		delete(c.hashIndex, key)
	}
}

// Read implements the io.Reader interface
func (r *chunkedReader) Read(p []byte) (n int, err error) {
	var length int
	for ; r.i < len(r.email.partsInfo.Parts); r.i++ {
		length, err = r.cache.warm(r.email.partsInfo.Parts[r.i].ChunkHash...)
		if err != nil {
			return
		}
		var nRead int
		for r.j < length {
			chunk, err := r.cache.get(r.j)
			if err != nil {
				return nRead, err
			}
			nRead, err = chunk.data.Read(p)
			if err == io.EOF { // we've read the entire chunk

				if closer, ok := chunk.data.(io.ReadCloser); ok {
					err = closer.Close()
					if err != nil {
						return nRead, err
					}
				}
				r.j++ // advance to the next chunk the part
				err = nil

				if r.j == length { // last chunk in a part?
					r.j = 0 // reset chunk index
					r.i++   // advance to the next part
					if r.i == len(r.email.partsInfo.Parts) || r.part > 0 {
						// there are no more parts to return
						err = io.EOF
						r.cache.empty()
					}
				}
			}

			// unless there's an error, the next time this function will be
			// called, it will read the next chunk
			return nRead, err
		}
	}
	err = io.EOF
	return n, err
}

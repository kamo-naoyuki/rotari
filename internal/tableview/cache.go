package tableview

import (
	"sync"
)

// cacheSize bounds how many indexes, listings, and results are kept; each
// is small next to the file it describes.
const cacheSize = 16

// cache keeps the most recently used values by key. A value is built
// outside the lock, so two requests may build the same one; the later
// simply replaces it.
type cache struct {
	mu    sync.Mutex
	order []string
	value map[string]any
}

var results = &cache{value: map[string]any{}}

func (c *cache) get(key string, build func() (any, error)) (any, error) {
	c.mu.Lock()
	if value, ok := c.value[key]; ok {
		c.touch(key)
		c.mu.Unlock()
		return value, nil
	}
	c.mu.Unlock()
	value, err := build()
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.value[key]; !ok && len(c.order) >= cacheSize {
		delete(c.value, c.order[0])
		c.order = c.order[1:]
	}
	c.value[key] = value
	c.touch(key)
	return value, nil
}

// touch moves key to the most recent end; the caller holds the lock.
func (c *cache) touch(key string) {
	for index, existing := range c.order {
		if existing == key {
			c.order = append(c.order[:index], c.order[index+1:]...)
			break
		}
	}
	c.order = append(c.order, key)
}

package golem

import (
	"github.com/expr-lang/expr/vm"
	lru "github.com/hashicorp/golang-lru/v2"
)

// programCache is a bounded, per-Engine cache of compiled programs keyed by the
// expression source string. It is a direct, thin use of
// github.com/hashicorp/golang-lru/v2, whose *lru.Cache is intrinsically
// goroutine-safe — golem adds NO extra sync.RWMutex around it (that would be
// redundant double-locking). The cache is never shared across Engines.
type programCache struct {
	lru *lru.Cache[string, *vm.Program]
}

// newProgramCache builds a cache holding up to size entries. A size <= 0 is
// treated as the default capacity.
func newProgramCache(size int) (*programCache, error) {
	if size <= 0 {
		size = defaultCacheSize
	}
	c, err := lru.New[string, *vm.Program](size)
	if err != nil {
		return nil, err
	}
	return &programCache{lru: c}, nil
}

// get returns the cached program for src, if present.
func (c *programCache) get(src string) (*vm.Program, bool) {
	return c.lru.Get(src)
}

// add stores prog under src, evicting the least-recently-used entry if the
// cache is full.
func (c *programCache) add(src string, prog *vm.Program) {
	c.lru.Add(src, prog)
}

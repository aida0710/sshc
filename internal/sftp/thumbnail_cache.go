package sftp

import (
	"container/list"
	"sync"
)

// maxThumbnailCacheBytes holds about 2000 thumbnails, more photos than a person scrolls through between engine
// restarts, in a small share of a phone's memory. Nothing is written to disk.
const maxThumbnailCacheBytes = 32 << 20

// thumbnailOutcome is what making a thumbnail came to: the picture, or the lasting refusal of a file that has none.
// Remembering a refusal keeps a file that cannot be shrunk from being read again whenever its row comes back on screen.
type thumbnailOutcome struct {
	thumbnail Thumbnail
	refusal   error
}

type thumbnailCacheEntry struct {
	key     string
	outcome thumbnailOutcome
}

// thumbnailCache keeps what was made, least recently used out first, within a byte budget. Its keys hold the file's
// revision, so a changed file has a new key and the old thumbnail ages out unused.
type thumbnailCache struct {
	mutex    sync.Mutex
	capacity int
	used     int
	order    *list.List
	entries  map[string]*list.Element
}

func newThumbnailCache(capacity int) *thumbnailCache {
	return &thumbnailCache{capacity: capacity, order: list.New(), entries: map[string]*list.Element{}}
}

func (c *thumbnailCache) get(key string) (thumbnailOutcome, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	element, found := c.entries[key]
	if !found {
		return thumbnailOutcome{}, false
	}
	c.order.MoveToFront(element)
	return element.Value.(*thumbnailCacheEntry).outcome, true
}

func (c *thumbnailCache) put(key string, outcome thumbnailOutcome) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if _, found := c.entries[key]; found {
		return
	}
	c.entries[key] = c.order.PushFront(&thumbnailCacheEntry{key: key, outcome: outcome})
	c.used += thumbnailCacheEntryBytes(key, outcome)
	for c.used > c.capacity {
		oldest := c.order.Back().Value.(*thumbnailCacheEntry)
		c.used -= thumbnailCacheEntryBytes(oldest.key, oldest.outcome)
		delete(c.entries, oldest.key)
		c.order.Remove(c.order.Back())
	}
}

func thumbnailCacheEntryBytes(key string, outcome thumbnailOutcome) int {
	return len(key) + len(outcome.thumbnail.Contents)
}

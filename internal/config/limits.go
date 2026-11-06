//go:build !wasip1

package config

// defaultMaxIndexFiles is 0, no limit: the native server indexes the whole
// workspace. Indexing all of Magento 2 (about 25,000 files) takes about 614 MB
// of heap. Users short on memory can set maxIndexFiles to cap it.
const defaultMaxIndexFiles = 0

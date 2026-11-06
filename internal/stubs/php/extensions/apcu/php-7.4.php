<?php

// Signatures adapted from the extension maintainers' own Zend API declarations.
// See internal/stubs/php/extensions/README.md for sources, API-version policy, and licenses.
/** @var int */ const APC_LIST_ACTIVE = 0x1;
/** @var int */ const APC_LIST_DELETED = 0x2;
/** @var int */ const APC_ITER_TYPE = (1 << 0);
/** @var int */ const APC_ITER_KEY = (1 << 1);
/** @var int */ const APC_ITER_VALUE = (1 << 2);
/** @var int */ const APC_ITER_NUM_HITS = (1 << 3);
/** @var int */ const APC_ITER_MTIME = (1 << 4);
/** @var int */ const APC_ITER_CTIME = (1 << 5);
/** @var int */ const APC_ITER_DTIME = (1 << 6);
/** @var int */ const APC_ITER_ATIME = (1 << 7);
/** @var int */ const APC_ITER_REFCOUNT = (1 << 8);
/** @var int */ const APC_ITER_MEM_SIZE = (1 << 9);
/** @var int */ const APC_ITER_TTL = (1 << 10);
/** @var int */ const APC_ITER_NONE = 0;
/** @var int */ const APC_ITER_ALL = (0xffffffff);
/** Remove all entries from the user cache. */ function apcu_clear_cache(): bool {}
/** Return cache statistics and optionally entry metadata. */ function apcu_cache_info(bool $limited = false): array|false {}
/** Return metadata for a cached key, or null when absent. */ function apcu_key_info(string $key): ?array {}
/** Return shared-memory allocation statistics. */ function apcu_sma_info(bool $limited = false): array|false {}
/** Report whether APCu is enabled for this process. */ function apcu_enabled(): bool {}
/** Store a value or key/value pairs; a bulk call returns failed keys. */ function apcu_store(string|array $key, mixed $value = null, int $ttl = 0): array|bool {}
/** Store values only for keys that do not already exist. */ function apcu_add(string|array $key, mixed $value = null, int $ttl = 0): array|bool {}
/** Atomically increment a cached integer. */ function apcu_inc(string $key, int $step = 1, ?bool &$success = null, int $ttl = 0): int|false {}
/** Atomically decrement a cached integer. */ function apcu_dec(string $key, int $step = 1, ?bool &$success = null, int $ttl = 0): int|false {}
/** Atomically replace an integer if it equals the expected old value. */ function apcu_cas(string $key, int $old, int $new): bool {}
/** Retrieve one or more values, reporting cache hits through success. */ function apcu_fetch(string|array $key, ?bool &$success = null): mixed {}
/** Test one key or return the subset of supplied keys that exist. */ function apcu_exists(string|array $key): array|bool {}
/** Delete a key, a list of keys, or entries selected by an iterator. */ function apcu_delete(APCUIterator|string|array $key): array|bool {}
/** Fetch an entry, computing and storing it atomically when absent. */ function apcu_entry(string $key, callable $callback, int $ttl = 0): mixed {}

/** APCUIterator extension API. */
class APCUIterator implements Iterator {
    /** Create an instance. */ public function __construct( array|string|null $search = null, int $format = APC_ITER_ALL, int $chunk_size = 0, int $list = APC_LIST_ACTIVE) {}
    /** Rewind. */ public function rewind(): void {}
    /** Next. */ public function next(): void {}
    /** Valid. */ public function valid(): bool {}
    /** Key. */ public function key(): string|int {}
    /** Current. */ public function current(): mixed {}
    /** Get total hits. */ public function getTotalHits(): int {}
    /** Get total size. */ public function getTotalSize(): int {}
    /** Get total count. */ public function getTotalCount(): int {}
}

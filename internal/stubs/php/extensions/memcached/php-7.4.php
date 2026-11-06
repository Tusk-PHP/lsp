<?php

// Signatures adapted from the extension maintainers' own Zend API declarations.
// See internal/stubs/php/extensions/README.md for sources, API-version policy, and licenses.
// TODO(verify): Constant values depend on the extension/library build.
// UNKNOWN is a non-executable placeholder; the declared type is authoritative.
/** Memcached extension API. */
class Memcached {
    public const int LIBMEMCACHED_VERSION_HEX = UNKNOWN;
    public const int OPT_COMPRESSION = UNKNOWN;
    public const int OPT_COMPRESSION_TYPE = UNKNOWN;
    public const int OPT_COMPRESSION_LEVEL = UNKNOWN;
    public const int OPT_PREFIX_KEY = UNKNOWN;
    public const int OPT_SERIALIZER = UNKNOWN;
    public const int OPT_USER_FLAGS = UNKNOWN;
    public const int OPT_STORE_RETRY_COUNT = UNKNOWN;
    public const int OPT_ITEM_SIZE_LIMIT = UNKNOWN;
    public const bool HAVE_IGBINARY = UNKNOWN;
    public const bool HAVE_ZSTD = UNKNOWN;
    public const bool HAVE_JSON = UNKNOWN;
    public const bool HAVE_MSGPACK = UNKNOWN;
    public const bool HAVE_ENCODING = UNKNOWN;
    public const bool HAVE_SESSION = UNKNOWN;
    public const bool HAVE_SASL = UNKNOWN;
    public const int OPT_HASH = UNKNOWN;
    public const int HASH_DEFAULT = UNKNOWN;
    public const int HASH_MD5 = UNKNOWN;
    public const int HASH_CRC = UNKNOWN;
    public const int HASH_FNV1_64 = UNKNOWN;
    public const int HASH_FNV1A_64 = UNKNOWN;
    public const int HASH_FNV1_32 = UNKNOWN;
    public const int HASH_FNV1A_32 = UNKNOWN;
    public const int HASH_HSIEH = UNKNOWN;
    public const int HASH_MURMUR = UNKNOWN;
    public const int OPT_DISTRIBUTION = UNKNOWN;
    public const int DISTRIBUTION_MODULA = UNKNOWN;
    public const int DISTRIBUTION_CONSISTENT = UNKNOWN;
    public const int DISTRIBUTION_VIRTUAL_BUCKET = UNKNOWN;
    public const int OPT_LIBKETAMA_COMPATIBLE = UNKNOWN;
    public const int OPT_LIBKETAMA_HASH = UNKNOWN;
    public const int OPT_TCP_KEEPALIVE = UNKNOWN;
    public const int OPT_BUFFER_WRITES = UNKNOWN;
    public const int OPT_BINARY_PROTOCOL = UNKNOWN;
    public const int OPT_NO_BLOCK = UNKNOWN;
    public const int OPT_TCP_NODELAY = UNKNOWN;
    public const int OPT_SOCKET_SEND_SIZE = UNKNOWN;
    public const int OPT_SOCKET_RECV_SIZE = UNKNOWN;
    public const int OPT_CONNECT_TIMEOUT = UNKNOWN;
    public const int OPT_RETRY_TIMEOUT = UNKNOWN;
    public const int OPT_DEAD_TIMEOUT = UNKNOWN;
    public const int OPT_SEND_TIMEOUT = UNKNOWN;
    public const int OPT_RECV_TIMEOUT = UNKNOWN;
    public const int OPT_POLL_TIMEOUT = UNKNOWN;
    public const int OPT_CACHE_LOOKUPS = UNKNOWN;
    public const int OPT_SERVER_FAILURE_LIMIT = UNKNOWN;
    public const int OPT_AUTO_EJECT_HOSTS = UNKNOWN;
    public const int OPT_HASH_WITH_PREFIX_KEY = UNKNOWN;
    public const int OPT_NOREPLY = UNKNOWN;
    public const int OPT_SORT_HOSTS = UNKNOWN;
    public const int OPT_VERIFY_KEY = UNKNOWN;
    public const int OPT_USE_UDP = UNKNOWN;
    public const int OPT_NUMBER_OF_REPLICAS = UNKNOWN;
    public const int OPT_RANDOMIZE_REPLICA_READ = UNKNOWN;
    public const int OPT_REMOVE_FAILED_SERVERS = UNKNOWN;
    public const int OPT_SERVER_TIMEOUT_LIMIT = UNKNOWN;
    public const int RES_SUCCESS = UNKNOWN;
    public const int RES_FAILURE = UNKNOWN;
    public const int RES_HOST_LOOKUP_FAILURE = UNKNOWN;
    public const int RES_CONNECTION_FAILURE = UNKNOWN;
    public const int RES_CONNECTION_BIND_FAILURE = UNKNOWN;
    public const int RES_WRITE_FAILURE = UNKNOWN;
    public const int RES_READ_FAILURE = UNKNOWN;
    public const int RES_UNKNOWN_READ_FAILURE = UNKNOWN;
    public const int RES_PROTOCOL_ERROR = UNKNOWN;
    public const int RES_CLIENT_ERROR = UNKNOWN;
    public const int RES_SERVER_ERROR = UNKNOWN;
    public const int RES_DATA_EXISTS = UNKNOWN;
    public const int RES_DATA_DOES_NOT_EXIST = UNKNOWN;
    public const int RES_NOTSTORED = UNKNOWN;
    public const int RES_STORED = UNKNOWN;
    public const int RES_NOTFOUND = UNKNOWN;
    public const int RES_PARTIAL_READ = UNKNOWN;
    public const int RES_SOME_ERRORS = UNKNOWN;
    public const int RES_NO_SERVERS = UNKNOWN;
    public const int RES_END = UNKNOWN;
    public const int RES_DELETED = UNKNOWN;
    public const int RES_VALUE = UNKNOWN;
    public const int RES_STAT = UNKNOWN;
    public const int RES_ITEM = UNKNOWN;
    public const int RES_ERRNO = UNKNOWN;
    public const int RES_FAIL_UNIX_SOCKET = UNKNOWN;
    public const int RES_NOT_SUPPORTED = UNKNOWN;
    public const int RES_NO_KEY_PROVIDED = UNKNOWN;
    public const int RES_FETCH_NOTFINISHED = UNKNOWN;
    public const int RES_TIMEOUT = UNKNOWN;
    public const int RES_BUFFERED = UNKNOWN;
    public const int RES_BAD_KEY_PROVIDED = UNKNOWN;
    public const int RES_INVALID_HOST_PROTOCOL = UNKNOWN;
    public const int RES_SERVER_MARKED_DEAD = UNKNOWN;
    public const int RES_UNKNOWN_STAT_KEY = UNKNOWN;
    public const int RES_INVALID_ARGUMENTS = UNKNOWN;
    public const int RES_PARSE_ERROR = UNKNOWN;
    public const int RES_PARSE_USER_ERROR = UNKNOWN;
    public const int RES_DEPRECATED = UNKNOWN;
    public const int RES_IN_PROGRESS = UNKNOWN;
    public const int RES_MAXIMUM_RETURN = UNKNOWN;
    public const int RES_MEMORY_ALLOCATION_FAILURE = UNKNOWN;
    public const int RES_CONNECTION_SOCKET_CREATE_FAILURE = UNKNOWN;
    public const int RES_E2BIG = UNKNOWN;
    public const int RES_KEY_TOO_BIG = UNKNOWN;
    public const int RES_SERVER_TEMPORARILY_DISABLED = UNKNOWN;
    public const int RES_SERVER_MEMORY_ALLOCATION_FAILURE = UNKNOWN;
    public const int RES_AUTH_PROBLEM = UNKNOWN;
    public const int RES_AUTH_FAILURE = UNKNOWN;
    public const int RES_AUTH_CONTINUE = UNKNOWN;
    public const int RES_PAYLOAD_FAILURE = UNKNOWN;
    public const int SERIALIZER_PHP = UNKNOWN;
    public const int SERIALIZER_IGBINARY = UNKNOWN;
    public const int SERIALIZER_JSON = UNKNOWN;
    public const int SERIALIZER_JSON_ARRAY = UNKNOWN;
    public const int SERIALIZER_MSGPACK = UNKNOWN;
    public const int COMPRESSION_FASTLZ = UNKNOWN;
    public const int COMPRESSION_ZLIB = UNKNOWN;
    public const int COMPRESSION_ZSTD = UNKNOWN;
    public const int GET_PRESERVE_ORDER = UNKNOWN;
    public const int GET_EXTENDED = UNKNOWN;
    public const int ON_CONNECT = UNKNOWN;
    public const int ON_ADD = UNKNOWN;
    public const int ON_APPEND = UNKNOWN;
    public const int ON_DECREMENT = UNKNOWN;
    public const int ON_DELETE = UNKNOWN;
    public const int ON_FLUSH = UNKNOWN;
    public const int ON_GET = UNKNOWN;
    public const int ON_INCREMENT = UNKNOWN;
    public const int ON_NOOP = UNKNOWN;
    public const int ON_PREPEND = UNKNOWN;
    public const int ON_QUIT = UNKNOWN;
    public const int ON_REPLACE = UNKNOWN;
    public const int ON_SET = UNKNOWN;
    public const int ON_STAT = UNKNOWN;
    public const int ON_VERSION = UNKNOWN;
    public const int RESPONSE_SUCCESS = UNKNOWN;
    public const int RESPONSE_KEY_ENOENT = UNKNOWN;
    public const int RESPONSE_KEY_EEXISTS = UNKNOWN;
    public const int RESPONSE_E2BIG = UNKNOWN;
    public const int RESPONSE_EINVAL = UNKNOWN;
    public const int RESPONSE_NOT_STORED = UNKNOWN;
    public const int RESPONSE_DELTA_BADVAL = UNKNOWN;
    public const int RESPONSE_NOT_MY_VBUCKET = UNKNOWN;
    public const int RESPONSE_AUTH_ERROR = UNKNOWN;
    public const int RESPONSE_AUTH_CONTINUE = UNKNOWN;
    public const int RESPONSE_UNKNOWN_COMMAND = UNKNOWN;
    public const int RESPONSE_ENOMEM = UNKNOWN;
    public const int RESPONSE_NOT_SUPPORTED = UNKNOWN;
    public const int RESPONSE_EINTERNAL = UNKNOWN;
    public const int RESPONSE_EBUSY = UNKNOWN;
    public const int RESPONSE_ETMPFAIL = UNKNOWN;
    public const bool GET_ERROR_RETURN_VALUE = UNKNOWN;
    /** Create an instance. */ public function __construct(?string $persistent_id=null, ?callable $callback=null, ?string $connection_str=null) {}
    /** Get result code. */ public function getResultCode(): int {}
    /** Get result message. */ public function getResultMessage(): string {}
    /** Get. */ public function get(string $key, ?callable $cache_cb=null, int $get_flags=0): mixed {}
    /** Get by key. */ public function getByKey(string $server_key, string $key, ?callable $cache_cb=null, int $get_flags=0): mixed {}
    /** Get multi. */ public function getMulti(array $keys, int $get_flags=0): array|false {}
    /** Get multi by key. */ public function getMultiByKey(string $server_key, array $keys, int $get_flags=0): array|false {}
    /** Get delayed. */ public function getDelayed(array $keys, bool $with_cas=false, ?callable $value_cb=null): bool {}
    /** Get delayed by key. */ public function getDelayedByKey(string $server_key, array $keys, bool $with_cas=false, ?callable $value_cb=null): bool {}
    /** Fetch. */ public function fetch(): array|false {}
    /** Fetch all. */ public function fetchAll(): array|false {}
    /** Set. */ public function set(string $key, mixed $value, int $expiration=0): bool {}
    /** Set by key. */ public function setByKey(string $server_key, string $key, mixed $value, int $expiration=0): bool {}
    /** Touch. */ public function touch(string $key, int $expiration=0): bool {}
    /** Touch by key. */ public function touchByKey(string $server_key, string $key, int $expiration=0): bool {}
    /** Set multi. */ public function setMulti(array $items, int $expiration=0): bool {}
    /** Set multi by key. */ public function setMultiByKey(string $server_key, array $items, int $expiration=0): bool {}
    /** Cas. */ public function cas(string|int|float $cas_token, string $key, mixed $value, int $expiration=0): bool {}
    /** Cas by key. */ public function casByKey(string|int|float $cas_token, string $server_key, string $key, mixed $value, int $expiration=0): bool {}
    /** Add. */ public function add(string $key, mixed $value, int $expiration=0): bool {}
    /** Add by key. */ public function addByKey(string $server_key, string $key, mixed $value, int $expiration=0): bool {}
    /** Append. */ public function append(string $key, string $value): ?bool {}
    /** Append by key. */ public function appendByKey(string $server_key, string $key, string $value): ?bool {}
    /** Prepend. */ public function prepend(string $key, string $value): ?bool {}
    /** Prepend by key. */ public function prependByKey(string $server_key, string $key, string $value): ?bool {}
    /** Replace. */ public function replace(string $key, mixed $value, int $expiration=0): bool {}
    /** Replace by key. */ public function replaceByKey(string $server_key, string $key, mixed $value, int $expiration=0): bool {}
    /** Delete. */ public function delete(string $key, int $time=0): bool {}
    /** Delete multi. */ public function deleteMulti(array $keys, int $time=0): array {}
    /** Delete by key. */ public function deleteByKey(string $server_key, string $key, int $time=0): bool {}
    /** Delete multi by key. */ public function deleteMultiByKey(string $server_key, array $keys, int $time=0): array {}
    /** Increment. */ public function increment(string $key, int $offset=1, int $initial_value=0, int $expiry=0): int|false {}
    /** Decrement. */ public function decrement(string $key, int $offset=1, int $initial_value=0, int $expiry=0): int|false {}
    /** Increment by key. */ public function incrementByKey(string $server_key, string $key, int $offset=1, int $initial_value=0, int $expiry=0): int|false {}
    /** Decrement by key. */ public function decrementByKey(string $server_key, string $key, int $offset=1, int $initial_value=0, int $expiry=0): int|false {}
    /** Add server. */ public function addServer(string $host, int $port, int $weight=0): bool {}
    /** Add servers. */ public function addServers(array $servers): bool {}
    /** Get server list. */ public function getServerList(): array {}
    /** Get server by key. */ public function getServerByKey(string $server_key): array|false {}
    /** Reset server list. */ public function resetServerList(): bool {}
    /** Quit. */ public function quit(): bool {}
    /** Flush buffers. */ public function flushBuffers(): bool {}
    /** Get last error message. */ public function getLastErrorMessage(): string {}
    /** Get last error code. */ public function getLastErrorCode(): int {}
    /** Get last error errno. */ public function getLastErrorErrno(): int {}
    /** Get last disconnected server. */ public function getLastDisconnectedServer(): array|false {}
    /** Get stats. */ public function getStats(?string $type=null): array|false {}
    /** Get version. */ public function getVersion(): array|false {}
    /** Get all keys. */ public function getAllKeys(): array|false {}
    /** Flush. */ public function flush(int $delay=0): bool {}
    /** Get option. */ public function getOption(int $option): mixed {}
    /** Set option. */ public function setOption(int $option, mixed $value): bool {}
    /** Set options. */ public function setOptions(array $options): bool {}
    /** Set bucket. */ public function setBucket(array $host_map, ?array $forward_map, int $replicas): bool {}
    /** Set sasl auth data. */ public function setSaslAuthData(string $username, string $password): bool {}
    /** Set encoding key. */ public function setEncodingKey(string $key): bool {}
    /** Is persistent. */ public function isPersistent(): bool {}
    /** Is pristine. */ public function isPristine(): bool {}
    /** Check key. */ public function checkKey(string $key): bool {}
}

/** Memcached operation failed. */
class MemcachedException extends RuntimeException {}

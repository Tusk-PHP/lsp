<?php

// Signatures adapted from the extension maintainers' own Zend API declarations.
// See internal/stubs/php/extensions/README.md for sources, API-version policy, and licenses.
/** Redis extension API. */
class Redis {
    public const REDIS_NOT_FOUND = 0;
    public const REDIS_STRING = 1;
    public const REDIS_SET = 2;
    public const REDIS_LIST = 3;
    public const REDIS_ZSET = 4;
    public const REDIS_HASH = 5;
    public const REDIS_STREAM = 6;
    public const ATOMIC = 0;
    public const MULTI = 1;
    public const PIPELINE = 2;
    public const OPT_SERIALIZER = 1;
    public const OPT_PREFIX = 2;
    public const OPT_READ_TIMEOUT = 3;
    public const OPT_TCP_KEEPALIVE = 6;
    public const OPT_COMPRESSION = 7;
    public const OPT_REPLY_LITERAL = 8;
    public const OPT_COMPRESSION_LEVEL = 9;
    public const OPT_NULL_MULTIBULK_AS_NULL = 10;
    public const SERIALIZER_NONE = 0;
    public const SERIALIZER_PHP = 1;
    public const SERIALIZER_IGBINARY = 2;
    public const SERIALIZER_MSGPACK = 3;
    public const SERIALIZER_JSON = 4;
    public const COMPRESSION_NONE = 0;
    public const COMPRESSION_LZF = 1;
    public const COMPRESSION_ZSTD = 2;
    // TODO(verify): Resolve the build-dependent value of COMPRESSION_ZSTD_DEFAULT.
    public const int COMPRESSION_ZSTD_DEFAULT = UNKNOWN;
    // TODO(verify): Resolve the build-dependent value of COMPRESSION_ZSTD_MAX.
    public const int COMPRESSION_ZSTD_MAX = UNKNOWN;
    public const COMPRESSION_LZ4 = 3;
    public const OPT_SCAN = 4;
    public const SCAN_RETRY = 1;
    public const SCAN_NORETRY = 0;
    public const SCAN_PREFIX = 2;
    public const SCAN_NOPREFIX = 3;
    public const OPT_MAX_RETRIES = 11;
    public const OPT_BACKOFF_ALGORITHM = 12;
    public const BACKOFF_ALGORITHM_DEFAULT = 0;
    public const BACKOFF_ALGORITHM_CONSTANT = 6;
    public const BACKOFF_ALGORITHM_UNIFORM = 5;
    public const BACKOFF_ALGORITHM_EXPONENTIAL = 4;
    public const BACKOFF_ALGORITHM_FULL_JITTER = 2;
    public const BACKOFF_ALGORITHM_EQUAL_JITTER = 3;
    public const BACKOFF_ALGORITHM_DECORRELATED_JITTER = 1;
    public const OPT_BACKOFF_BASE = 13;
    public const OPT_BACKOFF_CAP = 14;
    /** Create an instance. */ public function __construct(?array $options = null) {}
    /** Destruct. */ public function __destruct() {}
    /** Compress a value with the currently configured compressor as set with */ public function _compress(string $value): string {}
    /** Uncompress the provided argument that has been compressed with the */ public function _uncompress(string $value): string {}
    /** Prefix the passed argument with the currently set key prefix as set */ public function _prefix(string $key): string {}
    /** Serialize the provided value with the currently set serializer as set */ public function _serialize(mixed $value): string {}
    /** Unserialize the passed argument with the currently set serializer as set */ public function _unserialize(string $value): mixed {}
    /** Pack the provided value with the configured serializer and compressor */ public function _pack(mixed $value): string {}
    /** Unpack the provided value with the configured compressor and serializer */ public function _unpack(string $value): mixed {}
    /** Acl. */ public function acl(string $subcmd, string ...$args): mixed {}
    /** Append data to a Redis STRING key. */ public function append(string $key, mixed $value): Redis|int|false {}
    /** Authenticate a Redis connection after its been established. */ public function auth( mixed $credentials): Redis|bool {}
    /** Execute a save of the Redis database in the background. */ public function bgSave(): Redis|bool {}
    /** Asynchronously rewrite Redis' append-only file */ public function bgrewriteaof(): Redis|bool {}
    /** Count the number of set bits in a Redis string. */ public function bitcount(string $key, int $start = 0, int $end = -1, bool $bybit = false): Redis|int|false {}
    /** Bitop. */ public function bitop(string $operation, string $deskey, string $srckey, string ...$other_keys): Redis|int|false {}
    /** Return the position of the first bit set to 0 or 1 in a string. */ public function bitpos(string $key, bool $bit, int $start = 0, int $end = -1, bool $bybit = false): Redis|int|false {}
    /** Pop an element off the beginning of a Redis list or lists, potentially blocking up to a specified */ public function blPop(string|array $key_or_keys, string|float|int $timeout_or_key, mixed ...$extra_args): Redis|array|null|false {}
    /** Pop an element off of the end of a Redis list or lists, potentially blocking up to a specified timeout. */ public function brPop(string|array $key_or_keys, string|float|int $timeout_or_key, mixed ...$extra_args): Redis|array|null|false {}
    /** Pop an element from the end of a Redis list, pushing it to the beginning of another Redis list, */ public function brpoplpush(string $src, string $dst, int|float $timeout): Redis|string|false {}
    /** POP the maximum scoring element off of one or more sorted sets, blocking up to a specified */ public function bzPopMax(string|array $key, string|int $timeout_or_key, mixed ...$extra_args): Redis|array|false {}
    /** POP the minimum scoring element off of one or more sorted sets, blocking up to a specified timeout */ public function bzPopMin(string|array $key, string|int $timeout_or_key, mixed ...$extra_args): Redis|array|false {}
    /** POP one or more elements from one or more sorted sets, blocking up to a specified amount of time */ public function bzmpop(float $timeout, array $keys, string $from, int $count = 1): Redis|array|null|false {}
    /** POP one or more of the highest or lowest scoring elements from one or more sorted sets. */ public function zmpop(array $keys, string $from, int $count = 1): Redis|array|null|false {}
    /** Pop one or more elements from one or more Redis LISTs, blocking up to a specified timeout when */ public function blmpop(float $timeout, array $keys, string $from, int $count = 1): Redis|array|null|false {}
    /** Pop one or more elements off of one or more Redis LISTs. */ public function lmpop(array $keys, string $from, int $count = 1): Redis|array|null|false {}
    /** Reset any last error on the connection to NULL */ public function clearLastError(): bool {}
    /** Client. */ public function client(string $opt, mixed ...$args): mixed {}
    /** Close. */ public function close(): bool {}
    /** Command. */ public function command(?string $opt = null, mixed ...$args): mixed {}
    /** Execute the Redis CONFIG command in a variety of ways. */ public function config(string $operation, array|string|null $key_or_settings = null, ?string $value = null): mixed {}
    /** Connect. */ public function connect(string $host, int $port = 6379, float $timeout = 0, ?string $persistent_id = null, int $retry_interval = 0, float $read_timeout = 0, ?array $context = null): bool {}
    /** Make a copy of a key. */ public function copy(string $src, string $dst, ?array $options = null): Redis|bool {}
    /** Return the number of keys in the currently selected Redis database. */ public function dbSize(): Redis|int|false {}
    /** Debug. */ public function debug(string $key): Redis|string {}
    /** Decrement a Redis integer by 1 or a provided value. */ public function decr(string $key, int $by = 1): Redis|int|false {}
    /** Decrement a redis integer by a value */ public function decrBy(string $key, int $value): Redis|int|false {}
    /** Delete one or more keys from Redis. */ public function del(array|string $key, string ...$other_keys): Redis|int|false {}
    /** Delete. */ public function delete(array|string $key, string ...$other_keys): Redis|int|false {}
    /** Discard a transaction currently in progress. */ public function discard(): Redis|bool {}
    /** Dump Redis' internal binary representation of a key. */ public function dump(string $key): Redis|string {}
    /** Have Redis repeat back an arbitrary string to the client. */ public function echo(string $str): Redis|string|false {}
    /** Execute a LUA script on the redis server. */ public function eval(string $script, array $args = [], int $num_keys = 0): mixed {}
    /** This is simply the read-only variant of eval, meaning the underlying script */ public function eval_ro(string $script_sha, array $args = [], int $num_keys = 0): mixed {}
    /** Execute a LUA script on the server but instead of sending the script, send */ public function evalsha(string $sha1, array $args = [], int $num_keys = 0): mixed {}
    /** This is simply the read-only variant of evalsha, meaning the underlying script */ public function evalsha_ro(string $sha1, array $args = [], int $num_keys = 0): mixed {}
    /** Execute either a MULTI or PIPELINE block and return the array of replies. */ public function exec(): Redis|array|false {}
    /** Test if one or more keys exist. */ public function exists(mixed $key, mixed ...$other_keys): Redis|int|bool {}
    /** Sets an expiration in seconds on the key in question.  If connected to */ public function expire(string $key, int $timeout, ?string $mode = null): Redis|bool {}
    /** Set a key to expire at an exact unix timestamp. */ public function expireAt(string $key, int $timestamp, ?string $mode = null): Redis|bool {}
    /** Failover. */ public function failover(?array $to = null, bool $abort = false, int $timeout = 0): Redis|bool {}
    /** Get the expiration of a given key as a unix timestamp */ public function expiretime(string $key): Redis|int|false {}
    /** Get the expriation timestamp of a given Redis key but in milliseconds. */ public function pexpiretime(string $key): Redis|int|false {}
    /** Invoke a function. */ public function fcall(string $fn, array $keys = [], array $args = []): mixed {}
    /** This is a read-only variant of the FCALL command that cannot execute commands that modify data. */ public function fcall_ro(string $fn, array $keys = [], array $args = []): mixed {}
    /** Deletes every key in all Redis databases */ public function flushAll(?bool $sync = null): Redis|bool {}
    /** Deletes all the keys of the currently selected database. */ public function flushDB(?bool $sync = null): Redis|bool {}
    /** Functions is an API for managing code to be executed on the server. */ public function function(string $operation, mixed ...$args): Redis|bool|string|array {}
    /** Add one or more members to a geospacial sorted set */ public function geoadd(string $key, float $lng, float $lat, string $member, mixed ...$other_triples_and_options): Redis|int|false {}
    /** Get the distance between two members of a geospacially encoded sorted set. */ public function geodist(string $key, string $src, string $dst, ?string $unit = null): Redis|float|false {}
    /** Retrieve one or more GeoHash encoded strings for members of the set. */ public function geohash(string $key, string $member, string ...$other_members): Redis|array|false {}
    /** Return the longitude and lattitude for one or more members of a geospacially encoded sorted set. */ public function geopos(string $key, string $member, string ...$other_members): Redis|array|false {}
    /** Retrieve members of a geospacially sorted set that are within a certain radius of a location. */ public function georadius(string $key, float $lng, float $lat, float $radius, string $unit, array $options = []): mixed {}
    /** A readonly variant of `GEORADIUS` that may be executed on replicas. */ public function georadius_ro(string $key, float $lng, float $lat, float $radius, string $unit, array $options = []): mixed {}
    /** Similar to `GEORADIUS` except it uses a member as the center of the query. */ public function georadiusbymember(string $key, string $member, float $radius, string $unit, array $options = []): mixed {}
    /** This is the read-only variant of `GEORADIUSBYMEMBER` that can be run on replicas. */ public function georadiusbymember_ro(string $key, string $member, float $radius, string $unit, array $options = []): mixed {}
    /** Search a geospacial sorted set for members in various ways. */ public function geosearch(string $key, array|string $position, array|int|float $shape, string $unit, array $options = []): array {}
    /** Search a geospacial sorted set for members within a given area or range, storing the results into */ public function geosearchstore(string $dst, string $src, array|string $position, array|int|float $shape, string $unit, array $options = []): Redis|array|int|false {}
    /** Retrieve a string keys value. */ public function get(string $key): mixed {}
    /** Get the authentication information on the connection, if any. */ public function getAuth(): mixed {}
    /** Get the bit at a given index in a string key. */ public function getBit(string $key, int $idx): Redis|int|false {}
    /** Get the value of a key and optionally set it's expiration. */ public function getEx(string $key, array $options = []): Redis|string|bool {}
    /** Get the database number PhpRedis thinks we're connected to. */ public function getDBNum(): int {}
    /** Get a key from Redis and delete it in an atomic operation. */ public function getDel(string $key): Redis|string|bool {}
    /** Return the host or Unix socket we are connected to. */ public function getHost(): string {}
    /** Get the last error returned to us from Redis, if any. */ public function getLastError(): ?string {}
    /** Returns whether the connection is in ATOMIC, MULTI, or PIPELINE mode */ public function getMode(): int {}
    /** Retrieve the value of a configuration setting as set by Redis::setOption() */ public function getOption(int $option): mixed {}
    /** Get the persistent connection ID, if there is one. */ public function getPersistentID(): ?string {}
    /** Get the port we are connected to.  This number will be zero if we are connected to a unix socket. */ public function getPort(): int {}
    /** Retrieve a substring of a string by index. */ public function getRange(string $key, int $start, int $end): Redis|string|false {}
    /** Get the longest common subsequence between two string keys. */ public function lcs(string $key1, string $key2, ?array $options = null): Redis|string|array|int|false {}
    /** Get the currently set read timeout on the connection. */ public function getReadTimeout(): float {}
    /** Sets a key and returns any previously set value, if the key already existed. */ public function getset(string $key, mixed $value): Redis|string|false {}
    /** Retrieve any set connection timeout */ public function getTimeout(): float|false {}
    /** Get the number of bytes sent and received on the socket. */ public function getTransferredBytes(): array {}
    /** Reset the number of bytes sent and received on the socket. */ public function clearTransferredBytes(): void {}
    /** Remove one or more fields from a hash. */ public function hDel(string $key, string $field, string ...$other_fields): Redis|int|false {}
    /** Checks whether a field exists in a hash. */ public function hExists(string $key, string $field): Redis|bool {}
    /** H get. */ public function hGet(string $key, string $member): mixed {}
    /** Read every field and value from a hash. */ public function hGetAll(string $key): Redis|array|false {}
    /** Increment a hash field's value by an integer */ public function hIncrBy(string $key, string $field, int $value): Redis|int|false {}
    /** Increment a hash field by a floating point value */ public function hIncrByFloat(string $key, string $field, float $value): Redis|float|false {}
    /** Retrieve all of the fields of a hash. */ public function hKeys(string $key): Redis|array|false {}
    /** Get the number of fields in a hash. */ public function hLen(string $key): Redis|int|false {}
    /** Get one or more fields from a hash. */ public function hMget(string $key, array $fields): Redis|array|false {}
    /** Add or update one or more hash fields and values */ public function hMset(string $key, array $fieldvals): Redis|bool {}
    /** Get one or more random field from a hash. */ public function hRandField(string $key, ?array $options = null): Redis|string|array {}
    /** H set. */ public function hSet(string $key, string $member, mixed $value): Redis|int|false {}
    /** Set a hash field and value, but only if that field does not exist */ public function hSetNx(string $key, string $field, string $value): Redis|bool {}
    /** Get the string length of a hash field */ public function hStrLen(string $key, string $field): Redis|int|false {}
    /** Get all of the values from a hash. */ public function hVals(string $key): Redis|array|false {}
    /** Iterate over the fields and values of a hash in an incremental fashion. */ public function hscan(string $key, ?int &$iterator, ?string $pattern = null, int $count = 0): Redis|array|bool {}
    /** Increment a key's value, optionally by a specifc amount. */ public function incr(string $key, int $by = 1): Redis|int|false {}
    /** Increment a key by a specific integer value */ public function incrBy(string $key, int $value): Redis|int|false {}
    /** Increment a numeric key by a floating point value. */ public function incrByFloat(string $key, float $value): Redis|float|false {}
    /** Retrieve information about the connected redis-server.  If no arguments are passed to */ public function info(string ...$sections): Redis|array|false {}
    /** Check if we are currently connected to a Redis instance. */ public function isConnected(): bool {}
    /** Keys. */ public function keys(string $pattern): Redis|array|false {}
    /** L insert. */ public function lInsert(string $key, string $pos, mixed $pivot, mixed $value): Redis|int|false {}
    /** Retrieve the lenght of a list. */ public function lLen(string $key): Redis|int|false {}
    /** Move an element from one list into another. */ public function lMove(string $src, string $dst, string $wherefrom, string $whereto): Redis|string|false {}
    /** Move an element from one list to another, blocking up to a timeout until an element is available. */ public function blmove(string $src, string $dst, string $wherefrom, string $whereto, float $timeout): Redis|string|false {}
    /** Pop one or more elements off a list. */ public function lPop(string $key, int $count = 0): Redis|bool|string|array {}
    /** Retrieve the index of an element in a list. */ public function lPos(string $key, mixed $value, ?array $options = null): Redis|null|bool|int|array {}
    /** Prepend one or more elements to a list. */ public function lPush(string $key, mixed ...$elements): Redis|int|false {}
    /** Append one or more elements to a list. */ public function rPush(string $key, mixed ...$elements): Redis|int|false {}
    /** Prepend an element to a list but only if the list exists */ public function lPushx(string $key, mixed $value): Redis|int|false {}
    /** Append an element to a list but only if the list exists */ public function rPushx(string $key, mixed $value): Redis|int|false {}
    /** Set a list element at an index to a specific value. */ public function lSet(string $key, int $index, mixed $value): Redis|bool {}
    /** Retrieve the last time Redis' database was persisted to disk. */ public function lastSave(): int {}
    /** Get the element of a list by its index. */ public function lindex(string $key, int $index): mixed {}
    /** Retrieve elements from a list. */ public function lrange(string $key, int $start , int $end): Redis|array|false {}
    /** Remove one or more matching elements from a list. */ public function lrem(string $key, mixed $value, int $count = 0): Redis|int|false {}
    /** Trim a list to a subrange of elements. */ public function ltrim(string $key, int $start , int $end): Redis|bool {}
    /** Get one ore more string keys. */ public function mget(array $keys): Redis|array {}
    /** Migrate. */ public function migrate(string $host, int $port, string|array $key, int $dstdb, int $timeout, bool $copy = false, bool $replace = false, mixed $credentials = null): Redis|bool {}
    /** Move a key to a different database on the same redis instance. */ public function move(string $key, int $index): Redis|bool {}
    /** Set one ore more string keys. */ public function mset(array $key_values): Redis|bool {}
    /** Set one ore more string keys but only if none of the key exist. */ public function msetnx(array $key_values): Redis|bool {}
    /** Begin a transaction. */ public function multi(int $value = Redis::MULTI): bool|Redis {}
    /** Object. */ public function object(string $subcommand, string $key): Redis|int|string|false {}
    /** Open. */ public function open(string $host, int $port = 6379, float $timeout = 0, ?string $persistent_id = null, int $retry_interval = 0, float $read_timeout = 0, ?array $context = null): bool {}
    /** Pconnect. */ public function pconnect(string $host, int $port = 6379, float $timeout = 0, ?string $persistent_id = null, int $retry_interval = 0, float $read_timeout = 0, ?array $context = null): bool {}
    /** Remove the expiration from a key. */ public function persist(string $key): Redis|bool {}
    /** Sets an expiration in milliseconds on a given key.  If connected to Redis >= 7.0.0 */ public function pexpire(string $key, int $timeout, ?string $mode = null): bool {}
    /** Set a key's expiration to a specific Unix Timestamp in milliseconds.  If connected to */ public function pexpireAt(string $key, int $timestamp, ?string $mode = null): Redis|bool {}
    /** Add one or more elements to a Redis HyperLogLog key */ public function pfadd(string $key, array $elements): Redis|int {}
    /** Retrieve the cardinality of a Redis HyperLogLog key. */ public function pfcount(array|string $key_or_keys): Redis|int|false {}
    /** Merge one or more source HyperLogLog sets into a destination set. */ public function pfmerge(string $dst, array $srckeys): Redis|bool {}
    /** PING the redis server with an optional string argument. */ public function ping(?string $message = null): Redis|string|bool {}
    /** Enter into pipeline mode. */ public function pipeline(): bool|Redis {}
    /** Popen. */ public function popen(string $host, int $port = 6379, float $timeout = 0, ?string $persistent_id = null, int $retry_interval = 0, float $read_timeout = 0, ?array $context = null): bool {}
    /** Set a key with an expiration time in milliseconds */ public function psetex(string $key, int $expire, mixed $value): Redis|bool {}
    /** Subscribe to one or more glob-style patterns */ public function psubscribe(array $patterns, callable $cb): bool {}
    /** Get a keys time to live in milliseconds. */ public function pttl(string $key): Redis|int|false {}
    /** Publish a message to a pubsub channel */ public function publish(string $channel, string $message): Redis|int|false {}
    /** Pubsub. */ public function pubsub(string $command, mixed $arg = null): mixed {}
    /** Unsubscribe from one or more channels by pattern */ public function punsubscribe(array $patterns): Redis|array|bool {}
    /** Pop one or more elements from the end of a list. */ public function rPop(string $key, int $count = 0): Redis|array|string|bool {}
    /** Return a random key from the current database */ public function randomKey(): Redis|string|false {}
    /** Execute any arbitrary Redis command by name. */ public function rawcommand(string $command, mixed ...$args): mixed {}
    /** Unconditionally rename a key from $old_name to $new_name */ public function rename(string $old_name, string $new_name): Redis|bool {}
    /** Renames $key_src to $key_dst but only if newkey does not exist. */ public function renameNx(string $key_src, string $key_dst): Redis|bool {}
    /** Reset the state of the connection. */ public function reset(): Redis|bool {}
    /** Restore a key by the binary payload generated by the DUMP command. */ public function restore(string $key, int $ttl, string $value, ?array $options = null): Redis|bool {}
    /** Query whether the connected instance is a primary or replica */ public function role(): mixed {}
    /** Atomically pop an element off the end of a Redis LIST and push it to the beginning of */ public function rpoplpush(string $srckey, string $dstkey): Redis|string|false {}
    /** Add one or more values to a Redis SET key. */ public function sAdd(string $key, mixed $value, mixed ...$other_values): Redis|int|false {}
    /** Add one ore more values to a Redis SET key.  This is an alternative to Redis::sadd() but */ public function sAddArray(string $key, array $values): int {}
    /** Given one or more Redis SETS, this command returns all of the members from the first */ public function sDiff(string $key, string ...$other_keys): Redis|array|false {}
    /** This method performs the same operation as SDIFF except it stores the resulting diff */ public function sDiffStore(string $dst, string $key, string ...$other_keys): Redis|int|false {}
    /** Given one or more Redis SET keys, this command will return all of the elements that are */ public function sInter(array|string $key, string ...$other_keys): Redis|array|false {}
    /** Compute the intersection of one or more sets and return the cardinality of the result. */ public function sintercard(array $keys, int $limit = -1): Redis|int|false {}
    /** Perform the intersection of one or more Redis SETs, storing the result in a destination */ public function sInterStore(array|string $key, string ...$other_keys): Redis|int|false {}
    /** Retrieve every member from a set key. */ public function sMembers(string $key): Redis|array|false {}
    /** Check if one or more values are members of a set. */ public function sMisMember(string $key, string $member, string ...$other_members): Redis|array|false {}
    /** Pop a member from one set and push it onto another.  This command will create the */ public function sMove(string $src, string $dst, mixed $value): Redis|bool {}
    /** Remove one or more elements from a set. */ public function sPop(string $key, int $count = 0): Redis|string|array|false {}
    /** Retrieve one or more random members of a set. */ public function sRandMember(string $key, int $count = 0): Redis|string|array|false {}
    /** Returns the union of one or more Redis SET keys. */ public function sUnion(string $key, string ...$other_keys): Redis|array|false {}
    /** Perform a union of one or more Redis SET keys and store the result in a new set */ public function sUnionStore(string $dst, string $key, string ...$other_keys): Redis|int|false {}
    /** Persist the Redis database to disk.  This command will block the server until the save is */ public function save(): Redis|bool {}
    /** Incrementally scan the Redis keyspace, with optional pattern and type matching. */ public function scan(?int &$iterator, ?string $pattern = null, int $count = 0, ?string $type = null): array|false {}
    /** Retrieve the number of members in a Redis set. */ public function scard(string $key): Redis|int|false {}
    /** An administrative command used to interact with LUA scripts stored on the server. */ public function script(string $command, mixed ...$args): mixed {}
    /** Select a specific Redis database. */ public function select(int $db): Redis|bool {}
    /** Create or set a Redis STRING key to a value. */ public function set(string $key, mixed $value, mixed $options = null): Redis|string|bool {}
    /** Set a specific bit in a Redis string to zero or one */ public function setBit(string $key, int $idx, bool $value): Redis|int|false {}
    /** Update or append to a Redis string at a specific starting index */ public function setRange(string $key, int $index, string $value): Redis|int|false {}
    /** Set a configurable option on the Redis object. */ public function setOption(int $option, mixed $value): bool {}
    /** Set a Redis STRING key with a specific expiration in seconds. */ public function setex(string $key, int $expire, mixed $value): Redis|bool {}
    /** Set a key to a value, but only if that key does not already exist. */ public function setnx(string $key, mixed $value): Redis|bool {}
    /** Check whether a given value is the member of a Redis SET. */ public function sismember(string $key, mixed $value): Redis|bool {}
    /** Turn a redis instance into a replica of another or promote a replica */ public function slaveof(?string $host = null, int $port = 6379): Redis|bool {}
    /** Used to turn a Redis instance into a replica of another, or to remove */ public function replicaof(?string $host = null, int $port = 6379): Redis|bool {}
    /** Update one or more keys last modified metadata. */ public function touch(array|string $key_or_array, string ...$more_keys): Redis|int|false {}
    /** Interact with Redis' slowlog functionality in various ways, depending */ public function slowlog(string $operation, int $length = 0): mixed {}
    /** Sort the contents of a Redis key in various ways. */ public function sort(string $key, ?array $options = null): mixed {}
    /** This is simply a read-only variant of the sort command */ public function sort_ro(string $key, ?array $options = null): mixed {}
    /** Sort asc. */ public function sortAsc(string $key, ?string $pattern = null, mixed $get = null, int $offset = -1, int $count = -1, ?string $store = null): array {}
    /** Sort asc alpha. */ public function sortAscAlpha(string $key, ?string $pattern = null, mixed $get = null, int $offset = -1, int $count = -1, ?string $store = null): array {}
    /** Sort desc. */ public function sortDesc(string $key, ?string $pattern = null, mixed $get = null, int $offset = -1, int $count = -1, ?string $store = null): array {}
    /** Sort desc alpha. */ public function sortDescAlpha(string $key, ?string $pattern = null, mixed $get = null, int $offset = -1, int $count = -1, ?string $store = null): array {}
    /** Remove one or more values from a Redis SET key. */ public function srem(string $key, mixed $value, mixed ...$other_values): Redis|int|false {}
    /** Scan the members of a redis SET key. */ public function sscan(string $key, ?int &$iterator, ?string $pattern = null, int $count = 0): array|false {}
    /** Subscribes the client to the specified shard channels. */ public function ssubscribe(array $channels, callable $cb): bool {}
    /** Retrieve the length of a Redis STRING key. */ public function strlen(string $key): Redis|int|false {}
    /** Subscribe to one or more Redis pubsub channels. */ public function subscribe(array $channels, callable $cb): bool {}
    /** Unsubscribes the client from the given shard channels, */ public function sunsubscribe(array $channels): Redis|array|bool {}
    /** Atomically swap two Redis databases so that all of the keys in the source database will */ public function swapdb(int $src, int $dst): Redis|bool {}
    /** Retrieve the server time from the connected Redis instance. */ public function time(): Redis|array {}
    /** Get the amount of time a Redis key has before it will expire, in seconds. */ public function ttl(string $key): Redis|int|false {}
    /** Get the type of a given Redis key. */ public function type(string $key): Redis|int|false {}
    /** Delete one or more keys from the Redis database.  Unlike this operation, the actual */ public function unlink(array|string $key, string ...$other_keys): Redis|int|false {}
    /** Unsubscribe from one or more subscribed channels. */ public function unsubscribe(array $channels): Redis|array|bool {}
    /** Remove any previously WATCH'ed keys in a transaction. */ public function unwatch(): Redis|bool {}
    /** Watch one or more keys for conditional execution of a transaction. */ public function watch(array|string $key, string ...$other_keys): Redis|bool {}
    /** Block the client up to the provided timeout until a certain number of replicas have confirmed */ public function wait(int $numreplicas, int $timeout): int|false {}
    /** Acknowledge one ore more messages that are pending (have been consumed using XREADGROUP but */ public function xack(string $key, string $group, array $ids): int|false {}
    /** Append a message to a stream. */ public function xadd(string $key, string $id, array $values, int $maxlen = 0, bool $approx = false, bool $nomkstream = false): Redis|string|false {}
    /** This command allows a consumer to claim pending messages that have been idle for a specified period of time. */ public function xautoclaim(string $key, string $group, string $consumer, int $min_idle, string $start, int $count = -1, bool $justid = false): Redis|bool|array {}
    /** This method allows a consumer to take ownership of pending stream entries, by ID.  Another */ public function xclaim(string $key, string $group, string $consumer, int $min_idle, array $ids, array $options): Redis|array|bool {}
    /** Remove one or more specific IDs from a stream. */ public function xdel(string $key, array $ids): Redis|int|false {}
    /** XGROUP */ public function xgroup(string $operation, ?string $key = null, ?string $group = null, ?string $id_or_consumer = null, bool $mkstream = false, int $entries_read = -2): mixed {}
    /** Retrieve information about a stream key. */ public function xinfo(string $operation, ?string $arg1 = null, ?string $arg2 = null, int $count = -1): mixed {}
    /** Get the number of messages in a Redis STREAM key. */ public function xlen(string $key): Redis|int|false {}
    /** Interact with stream messages that have been consumed by a consumer group but not yet */ public function xpending(string $key, string $group, ?string $start = null, ?string $end = null, int $count = -1, ?string $consumer = null): Redis|array|false {}
    /** Get a range of entries from a STREAM key. */ public function xrange(string $key, string $start, string $end, int $count = -1): Redis|array|bool {}
    /** Consume one or more unconsumed elements in one or more streams. */ public function xread(array $streams, int $count = -1, int $block = -1): Redis|array|bool {}
    /** Read one or more messages using a consumer group. */ public function xreadgroup(string $group, string $consumer, array $streams, int $count = 1, int $block = 1): Redis|array|bool {}
    /** Get a range of entries from a STREAM ke in reverse cronological order. */ public function xrevrange(string $key, string $end, string $start, int $count = -1): Redis|array|bool {}
    /** Truncate a STREAM key in various ways. */ public function xtrim(string $key, string $threshold, bool $approx = false, bool $minid = false, int $limit = -1): Redis|int|false {}
    /** Add one or more elements and scores to a Redis sorted set. */ public function zAdd(string $key, array|float $score_or_options, mixed ...$more_scores_and_mems): Redis|int|float|false {}
    /** Return the number of elements in a sorted set. */ public function zCard(string $key): Redis|int|false {}
    /** Count the number of members in a sorted set with scores inside a provided range. */ public function zCount(string $key, string $start, string $end): Redis|int|false {}
    /** Create or increment the score of a member in a Redis sorted set */ public function zIncrBy(string $key, float $value, mixed $member): Redis|float|false {}
    /** Count the number of elements in a sorted set whos members fall within the provided */ public function zLexCount(string $key, string $min, string $max): Redis|int|false {}
    /** Retrieve the score of one or more members in a sorted set. */ public function zMscore(string $key, mixed $member, mixed ...$other_members): Redis|array|false {}
    /** Pop one or more of the highest scoring elements from a sorted set. */ public function zPopMax(string $key, ?int $count = null): Redis|array|false {}
    /** Pop one or more of the lowest scoring elements from a sorted set. */ public function zPopMin(string $key, ?int $count = null): Redis|array|false {}
    /** Retrieve a range of elements of a sorted set between a start and end point. */ public function zRange(string $key, string|int $start, string|int $end, array|bool|null $options = null): Redis|array|false {}
    /** Retrieve a range of elements from a sorted set by legographical range. */ public function zRangeByLex(string $key, string $min, string $max, int $offset = -1, int $count = -1): Redis|array|false {}
    /** Retrieve a range of members from a sorted set by their score. */ public function zRangeByScore(string $key, string $start, string $end, array $options = []): Redis|array|false {}
    /** This command is similar to ZRANGE except that instead of returning the values directly */ public function zrangestore(string $dstkey, string $srckey, string $start, string $end, array|bool|null $options = null): Redis|int|false {}
    /** Retrieve one or more random members from a Redis sorted set. */ public function zRandMember(string $key, ?array $options = null): Redis|string|array {}
    /** Get the rank of a member of a sorted set, by score. */ public function zRank(string $key, mixed $member): Redis|int|false {}
    /** Remove one or more members from a Redis sorted set. */ public function zRem(mixed $key, mixed $member, mixed ...$other_members): Redis|int|false {}
    /** Remove zero or more elements from a Redis sorted set by legographical range. */ public function zRemRangeByLex(string $key, string $min, string $max): Redis|int|false {}
    /** Remove one or more members of a sorted set by their rank. */ public function zRemRangeByRank(string $key, int $start, int $end): Redis|int|false {}
    /** Remove one or more members of a sorted set by their score. */ public function zRemRangeByScore(string $key, string $start, string $end): Redis|int|false {}
    /** List the members of a Redis sorted set in reverse order */ public function zRevRange(string $key, int $start, int $end, mixed $scores = null): Redis|array|false {}
    /** List members of a Redis sorted set within a legographical range, in reverse order. */ public function zRevRangeByLex(string $key, string $max, string $min, int $offset = -1, int $count = -1): Redis|array|false {}
    /** List elements from a Redis sorted set by score, highest to lowest */ public function zRevRangeByScore(string $key, string $max, string $min, array|bool $options = []): Redis|array|false {}
    /** Retrieve a member of a sorted set by reverse rank. */ public function zRevRank(string $key, mixed $member): Redis|int|false {}
    /** Get the score of a member of a sorted set. */ public function zScore(string $key, mixed $member): Redis|float|false {}
    /** Given one or more sorted set key names, return every element that is in the first */ public function zdiff(array $keys, ?array $options = null): Redis|array|false {}
    /** Store the difference of one or more sorted sets in a destination sorted set. */ public function zdiffstore(string $dst, array $keys): Redis|int|false {}
    /** Compute the intersection of one or more sorted sets and return the members */ public function zinter(array $keys, ?array $weights = null, ?array $options = null): Redis|array|false {}
    /** Similar to ZINTER but instead of returning the intersected values, this command returns the */ public function zintercard(array $keys, int $limit = -1): Redis|int|false {}
    /** Compute the intersection of one ore more sorted sets storing the result in a new sorted set. */ public function zinterstore(string $dst, array $keys, ?array $weights = null, ?string $aggregate = null): Redis|int|false {}
    /** Scan the members of a sorted set incrementally, using a cursor */ public function zscan(string $key, ?int &$iterator, ?string $pattern = null, int $count = 0): Redis|array|false {}
    /** Retrieve the union of one or more sorted sets */ public function zunion(array $keys, ?array $weights = null, ?array $options = null): Redis|array|false {}
    /** Perform a union on one or more Redis sets and store the result in a destination sorted set. */ public function zunionstore(string $dst, array $keys, ?array $weights = null, ?string $aggregate = null): Redis|int|false {}
}

/** RedisArray extension API. */
class RedisArray {
    /** Call. */ public function __call(string $function_name, array $arguments): mixed {}
    /** Create an instance. */ public function __construct(string|array $name_or_hosts, ?array $options = null) {}
    /** Continuum. */ public function _continuum(): bool|array {}
    /** Distributor. */ public function _distributor(): bool|callable {}
    /** Function. */ public function _function(): bool|callable {}
    /** Hosts. */ public function _hosts(): bool|array {}
    /** Instance. */ public function _instance(string $host): bool|null|Redis {}
    /** Rehash. */ public function _rehash(?callable $fn = null): bool|null {}
    /** Target. */ public function _target(string $key): bool|string|null {}
    /** Bgsave. */ public function bgsave(): array {}
    /** Del. */ public function del(string|array $key, string ...$otherkeys): bool|int {}
    /** Discard. */ public function discard(): bool|null {}
    /** Exec. */ public function exec(): bool|null {}
    /** Flushall. */ public function flushall(): bool|array {}
    /** Flushdb. */ public function flushdb(): bool|array {}
    /** Get option. */ public function getOption(int $opt): bool|array {}
    /** Hscan. */ public function hscan(string $key, ?int &$iterator, ?string $pattern = null, int $count = 0): bool|array {}
    /** Info. */ public function info(): bool|array {}
    /** Keys. */ public function keys(string $pattern): bool|array {}
    /** Mget. */ public function mget(array $keys): bool|array {}
    /** Mset. */ public function mset(array $pairs): bool {}
    /** Multi. */ public function multi(string $host, ?int $mode = null): bool|RedisArray {}
    /** Ping. */ public function ping(): bool|array {}
    /** Save. */ public function save(): bool|array {}
    /** Scan. */ public function scan(?int &$iterator, string $node, ?string $pattern = null, int $count = 0): bool|array {}
    /** Select. */ public function select(int $index): bool|array {}
    /** Set option. */ public function setOption(int $opt, string $value): bool|array {}
    /** Sscan. */ public function sscan(string $key, ?int &$iterator, ?string $pattern = null, int $count = 0): bool|array {}
    /** Unlink. */ public function unlink(string|array $key, string ...$otherkeys): bool|int {}
    /** Unwatch. */ public function unwatch(): bool|null {}
    /** Zscan. */ public function zscan(string $key, ?int &$iterator, ?string $pattern = null, int $count = 0): bool|array {}
}

/** RedisCluster extension API. */
class RedisCluster {
    public const OPT_SLAVE_FAILOVER = 5;
    public const FAILOVER_NONE = 0;
    public const FAILOVER_ERROR = 1;
    public const FAILOVER_DISTRIBUTE = 2;
    public const FAILOVER_DISTRIBUTE_SLAVES = 3;
    /** Create an instance. */ public function __construct(string|null $name, ?array $seeds = null, int|float $timeout = 0, int|float $read_timeout = 0, bool $persistent = false, mixed $auth = null, ?array $context = null) {}
    /** Compress. */ public function _compress(string $value): string {}
    /** Uncompress. */ public function _uncompress(string $value): string {}
    /** Serialize. */ public function _serialize(mixed $value): bool|string {}
    /** Unserialize. */ public function _unserialize(string $value): mixed {}
    /** Pack. */ public function _pack(mixed $value): string {}
    /** Unpack. */ public function _unpack(string $value): mixed {}
    /** Prefix. */ public function _prefix(string $key): bool|string {}
    /** Masters. */ public function _masters(): array {}
    /** Redir. */ public function _redir(): string|null {}
    /** Acl. */ public function acl(string|array $key_or_address, string $subcmd, string ...$args): mixed {}
    /** Append. */ public function append(string $key, mixed $value): RedisCluster|bool|int {}
    /** Bgrewriteaof. */ public function bgrewriteaof(string|array $key_or_address): RedisCluster|bool {}
    /** Bgsave. */ public function bgsave(string|array $key_or_address): RedisCluster|bool {}
    /** Bitcount. */ public function bitcount(string $key, int $start = 0, int $end = -1, bool $bybit = false): RedisCluster|bool|int {}
    /** Bitop. */ public function bitop(string $operation, string $deskey, string $srckey, string ...$otherkeys): RedisCluster|bool|int {}
    /** Return the position of the first bit set to 0 or 1 in a string. */ public function bitpos(string $key, bool $bit, int $start = 0, int $end = -1, bool $bybit = false): RedisCluster|int|false {}
    /** See Redis::blpop() */ public function blpop(string|array $key, string|float|int $timeout_or_key, mixed ...$extra_args): RedisCluster|array|null|false {}
    /** See Redis::brpop() */ public function brpop(string|array $key, string|float|int $timeout_or_key, mixed ...$extra_args): RedisCluster|array|null|false {}
    /** See Redis::brpoplpush() */ public function brpoplpush(string $srckey, string $deskey, int $timeout): mixed {}
    /** Move an element from one list into another. */ public function lmove(string $src, string $dst, string $wherefrom, string $whereto): Redis|string|false {}
    /** Move an element from one list to another, blocking up to a timeout until an element is available. */ public function blmove(string $src, string $dst, string $wherefrom, string $whereto, float $timeout): Redis|string|false {}
    /** Bzpopmax. */ public function bzpopmax(string|array $key, string|int $timeout_or_key, mixed ...$extra_args): array {}
    /** Bzpopmin. */ public function bzpopmin(string|array $key, string|int $timeout_or_key, mixed ...$extra_args): array {}
    /** Bzmpop. */ public function bzmpop(float $timeout, array $keys, string $from, int $count = 1): RedisCluster|array|null|false {}
    /** Zmpop. */ public function zmpop(array $keys, string $from, int $count = 1): RedisCluster|array|null|false {}
    /** Blmpop. */ public function blmpop(float $timeout, array $keys, string $from, int $count = 1): RedisCluster|array|null|false {}
    /** Lmpop. */ public function lmpop(array $keys, string $from, int $count = 1): RedisCluster|array|null|false {}
    /** Clearlasterror. */ public function clearlasterror(): bool {}
    /** Client. */ public function client(string|array $key_or_address, string $subcommand, ?string $arg = null): array|string|bool {}
    /** Close. */ public function close(): bool {}
    /** Cluster. */ public function cluster(string|array $key_or_address, string $command, mixed ...$extra_args): mixed {}
    /** Command. */ public function command(mixed ...$extra_args): mixed {}
    /** Config. */ public function config(string|array $key_or_address, string $subcommand, mixed ...$extra_args): mixed {}
    /** Dbsize. */ public function dbsize(string|array $key_or_address): RedisCluster|int {}
    /** Copy. */ public function copy(string $src, string $dst, ?array $options = null): RedisCluster|bool {}
    /** Decr. */ public function decr(string $key, int $by = 1): RedisCluster|int|false {}
    /** Decrby. */ public function decrby(string $key, int $value): RedisCluster|int|false {}
    /** Decrbyfloat. */ public function decrbyfloat(string $key, float $value): float {}
    /** Del. */ public function del(array|string $key, string ...$other_keys): RedisCluster|int|false {}
    /** Discard. */ public function discard(): bool {}
    /** Dump. */ public function dump(string $key): RedisCluster|string|false {}
    /** Echo. */ public function echo(string|array $key_or_address, string $msg): RedisCluster|string|false {}
    /** Eval. */ public function eval(string $script, array $args = [], int $num_keys = 0): mixed {}
    /** Eval ro. */ public function eval_ro(string $script, array $args = [], int $num_keys = 0): mixed {}
    /** Evalsha. */ public function evalsha(string $script_sha, array $args = [], int $num_keys = 0): mixed {}
    /** Evalsha ro. */ public function evalsha_ro(string $script_sha, array $args = [], int $num_keys = 0): mixed {}
    /** Exec. */ public function exec(): array|false {}
    /** Exists. */ public function exists(mixed $key, mixed ...$other_keys): RedisCluster|int|bool {}
    /** Touch. */ public function touch(mixed $key, mixed ...$other_keys): RedisCluster|int|bool {}
    /** Expire. */ public function expire(string $key, int $timeout, ?string $mode = null): RedisCluster|bool {}
    /** Expireat. */ public function expireat(string $key, int $timestamp, ?string $mode = null): RedisCluster|bool {}
    /** Expiretime. */ public function expiretime(string $key): RedisCluster|int|false {}
    /** Pexpiretime. */ public function pexpiretime(string $key): RedisCluster|int|false {}
    /** Flushall. */ public function flushall(string|array $key_or_address, bool $async = false): RedisCluster|bool {}
    /** Flushdb. */ public function flushdb(string|array $key_or_address, bool $async = false): RedisCluster|bool {}
    /** Geoadd. */ public function geoadd(string $key, float $lng, float $lat, string $member, mixed ...$other_triples_and_options): RedisCluster|int|false {}
    /** Geodist. */ public function geodist(string $key, string $src, string $dest, ?string $unit = null): RedisCluster|float|false {}
    /** Geohash. */ public function geohash(string $key, string $member, string ...$other_members): RedisCluster|array|false {}
    /** Geopos. */ public function geopos(string $key, string $member, string ...$other_members): RedisCluster|array|false {}
    /** Georadius. */ public function georadius(string $key, float $lng, float $lat, float $radius, string $unit, array $options = []): mixed {}
    /** Georadius ro. */ public function georadius_ro(string $key, float $lng, float $lat, float $radius, string $unit, array $options = []): mixed {}
    /** Georadiusbymember. */ public function georadiusbymember(string $key, string $member, float $radius, string $unit, array $options = []): mixed {}
    /** Georadiusbymember ro. */ public function georadiusbymember_ro(string $key, string $member, float $radius, string $unit, array $options = []): mixed {}
    /** Geosearch. */ public function geosearch(string $key, array|string $position, array|int|float $shape, string $unit, array $options = []): RedisCluster|array {}
    /** Geosearchstore. */ public function geosearchstore(string $dst, string $src, array|string $position, array|int|float $shape, string $unit, array $options = []): RedisCluster|array|int|false {}
    /** Get. */ public function get(string $key): mixed {}
    /** Getbit. */ public function getbit(string $key, int $value): RedisCluster|int|false {}
    /** Getlasterror. */ public function getlasterror(): string|null {}
    /** Getmode. */ public function getmode(): int {}
    /** Getoption. */ public function getoption(int $option): mixed {}
    /** Getrange. */ public function getrange(string $key, int $start, int $end): RedisCluster|string|false {}
    /** Lcs. */ public function lcs(string $key1, string $key2, ?array $options = null): RedisCluster|string|array|int|false {}
    /** Getset. */ public function getset(string $key, mixed $value): RedisCluster|string|bool {}
    /** Gettransferredbytes. */ public function gettransferredbytes(): array|false {}
    /** Cleartransferredbytes. */ public function cleartransferredbytes(): void {}
    /** Hdel. */ public function hdel(string $key, string $member, string ...$other_members): RedisCluster|int|false {}
    /** Hexists. */ public function hexists(string $key, string $member): RedisCluster|bool {}
    /** Hget. */ public function hget(string $key, string $member): mixed {}
    /** Hgetall. */ public function hgetall(string $key): RedisCluster|array|false {}
    /** Hincrby. */ public function hincrby(string $key, string $member, int $value): RedisCluster|int|false {}
    /** Hincrbyfloat. */ public function hincrbyfloat(string $key, string $member, float $value): RedisCluster|float|false {}
    /** Hkeys. */ public function hkeys(string $key): RedisCluster|array|false {}
    /** Hlen. */ public function hlen(string $key): RedisCluster|int|false {}
    /** Hmget. */ public function hmget(string $key, array $keys): RedisCluster|array|false {}
    /** Hmset. */ public function hmset(string $key, array $key_values): RedisCluster|bool {}
    /** Hscan. */ public function hscan(string $key, ?int &$iterator, ?string $pattern = null, int $count = 0): array|bool {}
    /** Hrandfield. */ public function hrandfield(string $key, ?array $options = null): RedisCluster|string|array {}
    /** Hset. */ public function hset(string $key, string $member, mixed $value): RedisCluster|int|false {}
    /** Hsetnx. */ public function hsetnx(string $key, string $member, mixed $value): RedisCluster|bool {}
    /** Hstrlen. */ public function hstrlen(string $key, string $field): RedisCluster|int|false {}
    /** Hvals. */ public function hvals(string $key): RedisCluster|array|false {}
    /** Incr. */ public function incr(string $key, int $by = 1): RedisCluster|int|false {}
    /** Incrby. */ public function incrby(string $key, int $value): RedisCluster|int|false {}
    /** Incrbyfloat. */ public function incrbyfloat(string $key, float $value): RedisCluster|float|false {}
    /** Retrieve information about the connected redis-server.  If no arguments are passed to */ public function info(string|array $key_or_address, string ...$sections): RedisCluster|array|false {}
    /** Keys. */ public function keys(string $pattern): RedisCluster|array|false {}
    /** Lastsave. */ public function lastsave(string|array $key_or_address): RedisCluster|int|false {}
    /** Lget. */ public function lget(string $key, int $index): RedisCluster|string|bool {}
    /** Lindex. */ public function lindex(string $key, int $index): mixed {}
    /** Linsert. */ public function linsert(string $key, string $pos, mixed $pivot, mixed $value): RedisCluster|int|false {}
    /** Llen. */ public function llen(string $key): RedisCluster|int|bool {}
    /** Lpop. */ public function lpop(string $key, int $count = 0): RedisCluster|bool|string|array {}
    /** Lpos. */ public function lpos(string $key, mixed $value, ?array $options = null): Redis|null|bool|int|array {}
    /** Lpush. */ public function lpush(string $key, mixed $value, mixed ...$other_values): RedisCluster|int|bool {}
    /** Lpushx. */ public function lpushx(string $key, mixed $value): RedisCluster|int|bool {}
    /** Lrange. */ public function lrange(string $key, int $start, int $end): RedisCluster|array|false {}
    /** Lrem. */ public function lrem(string $key, mixed $value, int $count = 0): RedisCluster|int|bool {}
    /** Lset. */ public function lset(string $key, int $index, mixed $value): RedisCluster|bool {}
    /** Ltrim. */ public function ltrim(string $key, int $start, int $end): RedisCluster|bool {}
    /** Mget. */ public function mget(array $keys): RedisCluster|array|false {}
    /** Mset. */ public function mset(array $key_values): RedisCluster|bool {}
    /** Msetnx. */ public function msetnx(array $key_values): RedisCluster|array|false {}
    /** Multi. */ public function multi(int $value = Redis::MULTI): RedisCluster|bool {}
    /** Object. */ public function object(string $subcommand, string $key): RedisCluster|int|string|false {}
    /** Persist. */ public function persist(string $key): RedisCluster|bool {}
    /** Pexpire. */ public function pexpire(string $key, int $timeout, ?string $mode = null): RedisCluster|bool {}
    /** Pexpireat. */ public function pexpireat(string $key, int $timestamp, ?string $mode = null): RedisCluster|bool {}
    /** Pfadd. */ public function pfadd(string $key, array $elements): RedisCluster|bool {}
    /** Pfcount. */ public function pfcount(string $key): RedisCluster|int|false {}
    /** Pfmerge. */ public function pfmerge(string $key, array $keys): RedisCluster|bool {}
    /** PING an instance in the redis cluster. */ public function ping(string|array $key_or_address, ?string $message = null): mixed {}
    /** Psetex. */ public function psetex(string $key, int $timeout, string $value): RedisCluster|bool {}
    /** Psubscribe. */ public function psubscribe(array $patterns, callable $callback): void {}
    /** Pttl. */ public function pttl(string $key): RedisCluster|int|false {}
    /** Publish. */ public function publish(string $channel, string $message): RedisCluster|bool {}
    /** Pubsub. */ public function pubsub(string|array $key_or_address, string ...$values): mixed {}
    /** Punsubscribe. */ public function punsubscribe(string $pattern, string ...$other_patterns): bool|array {}
    /** Randomkey. */ public function randomkey(string|array $key_or_address): RedisCluster|bool|string {}
    /** Rawcommand. */ public function rawcommand(string|array $key_or_address, string $command, mixed ...$args): mixed {}
    /** Rename. */ public function rename(string $key_src, string $key_dst): RedisCluster|bool {}
    /** Renamenx. */ public function renamenx(string $key, string $newkey): RedisCluster|bool {}
    /** Restore. */ public function restore(string $key, int $timeout, string $value, ?array $options = null): RedisCluster|bool {}
    /** Role. */ public function role(string|array $key_or_address): mixed {}
    /** Rpop. */ public function rpop(string $key, int $count = 0): RedisCluster|bool|string|array {}
    /** Rpoplpush. */ public function rpoplpush(string $src, string $dst): RedisCluster|bool|string {}
    /** Rpush. */ public function rpush(string $key, mixed ...$elements): RedisCluster|int|false {}
    /** Rpushx. */ public function rpushx(string $key, string $value): RedisCluster|bool|int {}
    /** Sadd. */ public function sadd(string $key, mixed $value, mixed ...$other_values): RedisCluster|int|false {}
    /** Saddarray. */ public function saddarray(string $key, array $values): RedisCluster|bool|int {}
    /** Save. */ public function save(string|array $key_or_address): RedisCluster|bool {}
    /** Scan. */ public function scan(?int &$iterator, string|array $key_or_address, ?string $pattern = null, int $count = 0): bool|array {}
    /** Scard. */ public function scard(string $key): RedisCluster|int|false {}
    /** Script. */ public function script(string|array $key_or_address, mixed ...$args): mixed {}
    /** Sdiff. */ public function sdiff(string $key, string ...$other_keys): RedisCluster|array|false {}
    /** Sdiffstore. */ public function sdiffstore(string $dst, string $key, string ...$other_keys): RedisCluster|int|false {}
    /** Set. */ public function set(string $key, mixed $value, mixed $options = null): RedisCluster|string|bool {}
    /** Setbit. */ public function setbit(string $key, int $offset, bool $onoff): RedisCluster|int|false {}
    /** Setex. */ public function setex(string $key, int $expire, mixed $value): RedisCluster|bool {}
    /** Setnx. */ public function setnx(string $key, mixed $value): RedisCluster|bool {}
    /** Setoption. */ public function setoption(int $option, mixed $value): bool {}
    /** Setrange. */ public function setrange(string $key, int $offset, string $value): RedisCluster|int|false {}
    /** Sinter. */ public function sinter(array|string $key, string ...$other_keys): RedisCluster|array|false {}
    /** Sintercard. */ public function sintercard(array $keys, int $limit = -1): RedisCluster|int|false {}
    /** Sinterstore. */ public function sinterstore(array|string $key, string ...$other_keys): RedisCluster|int|false {}
    /** Sismember. */ public function sismember(string $key, mixed $value): RedisCluster|bool {}
    /** Smismember. */ public function smismember(string $key, string $member, string ...$other_members): RedisCluster|array|false {}
    /** Slowlog. */ public function slowlog(string|array $key_or_address, mixed ...$args): mixed {}
    /** Smembers. */ public function smembers(string $key): RedisCluster|array|false {}
    /** Smove. */ public function smove(string $src, string $dst, string $member): RedisCluster|bool {}
    /** Sort. */ public function sort(string $key, ?array $options = null): RedisCluster|array|bool|int|string {}
    /** Sort ro. */ public function sort_ro(string $key, ?array $options = null): RedisCluster|array|bool|int|string {}
    /** Spop. */ public function spop(string $key, int $count = 0): RedisCluster|string|array|false {}
    /** Srandmember. */ public function srandmember(string $key, int $count = 0): RedisCluster|string|array|false {}
    /** Srem. */ public function srem(string $key, mixed $value, mixed ...$other_values): RedisCluster|int|false {}
    /** Sscan. */ public function sscan(string $key, ?int &$iterator, ?string $pattern = null, int $count = 0): array|false {}
    /** Strlen. */ public function strlen(string $key): RedisCluster|int|false {}
    /** Subscribe. */ public function subscribe(array $channels, callable $cb): void {}
    /** Sunion. */ public function sunion(string $key, string ...$other_keys): RedisCluster|bool|array {}
    /** Sunionstore. */ public function sunionstore(string $dst, string $key, string ...$other_keys): RedisCluster|int|false {}
    /** Time. */ public function time(string|array $key_or_address): RedisCluster|bool|array {}
    /** Ttl. */ public function ttl(string $key): RedisCluster|int|false {}
    /** Type. */ public function type(string $key): RedisCluster|int|false {}
    /** Unsubscribe. */ public function unsubscribe(array $channels): bool|array {}
    /** Unlink. */ public function unlink(array|string $key, string ...$other_keys): RedisCluster|int|false {}
    /** Unwatch. */ public function unwatch(): bool {}
    /** Watch. */ public function watch(string $key, string ...$other_keys): RedisCluster|bool {}
    /** Xack. */ public function xack(string $key, string $group, array $ids): RedisCluster|int|false {}
    /** Xadd. */ public function xadd(string $key, string $id, array $values, int $maxlen = 0, bool $approx = false): RedisCluster|string|false {}
    /** Xclaim. */ public function xclaim(string $key, string $group, string $consumer, int $min_iddle, array $ids, array $options): RedisCluster|string|array|false {}
    /** Xdel. */ public function xdel(string $key, array $ids): RedisCluster|int|false {}
    /** Xgroup. */ public function xgroup(string $operation, ?string $key = null, ?string $group = null, ?string $id_or_consumer = null, bool $mkstream = false, int $entries_read = -2): mixed {}
    /** Xautoclaim. */ public function xautoclaim(string $key, string $group, string $consumer, int $min_idle, string $start, int $count = -1, bool $justid = false): RedisCluster|bool|array {}
    /** Xinfo. */ public function xinfo(string $operation, ?string $arg1 = null, ?string $arg2 = null, int $count = -1): mixed {}
    /** Xlen. */ public function xlen(string $key): RedisCluster|int|false {}
    /** Xpending. */ public function xpending(string $key, string $group, ?string $start = null, ?string $end = null, int $count = -1, ?string $consumer = null): RedisCluster|array|false {}
    /** Xrange. */ public function xrange(string $key, string $start, string $end, int $count = -1): RedisCluster|bool|array {}
    /** Xread. */ public function xread(array $streams, int $count = -1, int $block = -1): RedisCluster|bool|array {}
    /** Xreadgroup. */ public function xreadgroup(string $group, string $consumer, array $streams, int $count = 1, int $block = 1): RedisCluster|bool|array {}
    /** Xrevrange. */ public function xrevrange(string $key, string $start, string $end, int $count = -1): RedisCluster|bool|array {}
    /** Xtrim. */ public function xtrim(string $key, int $maxlen, bool $approx = false, bool $minid = false, int $limit = -1): RedisCluster|int|false {}
    /** Zadd. */ public function zadd(string $key, array|float $score_or_options, mixed ...$more_scores_and_mems): RedisCluster|int|float|false {}
    /** Zcard. */ public function zcard(string $key): RedisCluster|int|false {}
    /** Zcount. */ public function zcount(string $key, string $start, string $end): RedisCluster|int|false {}
    /** Zincrby. */ public function zincrby(string $key, float $value, string $member): RedisCluster|float|false {}
    /** Zinterstore. */ public function zinterstore(string $dst, array $keys, ?array $weights = null, ?string $aggregate = null): RedisCluster|int|false {}
    /** Zintercard. */ public function zintercard(array $keys, int $limit = -1): RedisCluster|int|false {}
    /** Zlexcount. */ public function zlexcount(string $key, string $min, string $max): RedisCluster|int|false {}
    /** Zpopmax. */ public function zpopmax(string $key, ?int $value = null): RedisCluster|bool|array {}
    /** Zpopmin. */ public function zpopmin(string $key, ?int $value = null): RedisCluster|bool|array {}
    /** Zrange. */ public function zrange(string $key, mixed $start, mixed $end, array|bool|null $options = null): RedisCluster|array|bool {}
    /** Zrangestore. */ public function zrangestore(string $dstkey, string $srckey, int $start, int $end, array|bool|null $options = null): RedisCluster|int|false {}
    /** Zrandmember. */ public function zrandmember(string $key, ?array $options = null): RedisCluster|string|array {}
    /** Zrangebylex. */ public function zrangebylex(string $key, string $min, string $max, int $offset = -1, int $count = -1): RedisCluster|array|false {}
    /** Zrangebyscore. */ public function zrangebyscore(string $key, string $start, string $end, array $options = []): RedisCluster|array|false {}
    /** Zrank. */ public function zrank(string $key, mixed $member): RedisCluster|int|false {}
    /** Zrem. */ public function zrem(string $key, string $value, string ...$other_values): RedisCluster|int|false {}
    /** Zremrangebylex. */ public function zremrangebylex(string $key, string $min, string $max): RedisCluster|int|false {}
    /** Zremrangebyrank. */ public function zremrangebyrank(string $key, string $min, string $max): RedisCluster|int|false {}
    /** Zremrangebyscore. */ public function zremrangebyscore(string $key, string $min, string $max): RedisCluster|int|false {}
    /** Zrevrange. */ public function zrevrange(string $key, string $min, string $max, ?array $options = null): RedisCluster|bool|array {}
    /** Zrevrangebylex. */ public function zrevrangebylex(string $key, string $min, string $max, ?array $options = null): RedisCluster|bool|array {}
    /** Zrevrangebyscore. */ public function zrevrangebyscore(string $key, string $min, string $max, ?array $options = null): RedisCluster|bool|array {}
    /** Zrevrank. */ public function zrevrank(string $key, mixed $member): RedisCluster|int|false {}
    /** Zscan. */ public function zscan(string $key, ?int &$iterator, ?string $pattern = null, int $count = 0): RedisCluster|bool|array {}
    /** Zscore. */ public function zscore(string $key, mixed $member): RedisCluster|float|false {}
    /** Zmscore. */ public function zmscore(string $key, mixed $member, mixed ...$other_members): Redis|array|false {}
    /** Zunionstore. */ public function zunionstore(string $dst, array $keys, ?array $weights = null, ?string $aggregate = null): RedisCluster|int|false {}
    /** Zinter. */ public function zinter(array $keys, ?array $weights = null, ?array $options = null): RedisCluster|array|false {}
    /** Zdiffstore. */ public function zdiffstore(string $dst, array $keys): RedisCluster|int|false {}
    /** Zunion. */ public function zunion(array $keys, ?array $weights = null, ?array $options = null): RedisCluster|array|false {}
    /** Zdiff. */ public function zdiff(array $keys, ?array $options = null): RedisCluster|array|false {}
}

/** RedisSentinel extension API. */
class RedisSentinel {
    /** Create an instance. */ public function __construct(?array $options = null) {}
    /** Ckquorum. */ public function ckquorum(string $master): bool|RedisSentinel {}
    /** Failover. */ public function failover(string $master): bool|RedisSentinel {}
    /** Flushconfig. */ public function flushconfig(): bool|RedisSentinel {}
    /** Get master addr by name. */ public function getMasterAddrByName(string $master): array|bool|RedisSentinel {}
    /** Master. */ public function master(string $master): array|bool|RedisSentinel {}
    /** Masters. */ public function masters(): array|bool|RedisSentinel {}
    /** Myid. */ public function myid(): string {}
    /** Ping. */ public function ping(): bool|RedisSentinel {}
    /** Reset. */ public function reset(string $pattern): int|RedisSentinel {}
    /** Sentinels. */ public function sentinels(string $master): array|bool|RedisSentinel {}
    /** Slaves. */ public function slaves(string $master): array|bool|RedisSentinel {}
}

/** Redis operation failed. */
class RedisException extends RuntimeException {}

/** Redis Cluster operation failed. */
class RedisClusterException extends RuntimeException {}

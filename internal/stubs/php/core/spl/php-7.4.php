<?php

// PHP 7.4 baseline for the iteration and array-access classes, with the
// generic PHPDoc PHPStan gives them, so foreach and offsets over them are
// typed. Signatures from PHP's own reflection.

/**
 * @template-covariant TKey
 * @template-covariant TValue
 */
interface Traversable
{
}

/**
 * @template-covariant TKey
 * @template-covariant TValue
 * @extends \Traversable<TKey, TValue>
 */
interface Iterator extends \Traversable
{
    /** @return TValue */
    public function current();

    /** @return TKey */
    public function key();

    public function next();

    public function rewind();

    public function valid();
}

/**
 * @template-covariant TKey
 * @template-covariant TValue
 * @extends \Traversable<TKey, TValue>
 */
interface IteratorAggregate extends \Traversable
{
    /** @return \Traversable<TKey, TValue> */
    public function getIterator();
}

/**
 * @template TKey
 * @template TValue
 */
interface ArrayAccess
{
    /** @param TKey $offset */
    public function offsetExists($offset);

    /**
     * @param TKey $offset
     * @return TValue
     */
    public function offsetGet($offset);

    /**
     * @param TKey|null $offset
     * @param TValue $value
     */
    public function offsetSet($offset, $value);

    /** @param TKey $offset */
    public function offsetUnset($offset);
}

/**
 * @template-covariant TKey
 * @template-covariant TValue
 * @template TSend
 * @template-covariant TReturn
 * @implements \Iterator<TKey, TValue>
 */
final class Generator implements \Iterator
{
    /** @return TValue */
    public function current(): mixed {}

    /** @return TKey */
    public function key(): mixed {}

    public function next(): void {}

    public function rewind(): void {}

    /**
     * @param TSend $value
     * @return TValue
     */
    public function send(mixed $value): mixed {}

    /** @return TValue */
    public function throw(\Throwable $exception): mixed {}

    public function valid(): bool {}

    /** @return TReturn */
    public function getReturn(): mixed {}

    public function __wakeup(): void {}
}

/**
 * @template TKey of array-key
 * @template TValue
 * @implements \Iterator<TKey, TValue>
 * @implements \ArrayAccess<TKey, TValue>
 */
class ArrayIterator implements \SeekableIterator, \ArrayAccess, \Serializable, \Countable, \Iterator, \Traversable
{
    public const STD_PROP_LIST = 1;

    public const ARRAY_AS_PROPS = 2;

    /** @param array<TKey, TValue>|object $array */
    public function __construct(array|object $array = [], int $flags = 0) {}

    /** @param TKey $key */
    public function offsetExists(mixed $key): bool {}

    /**
     * @param TKey $key
     * @return TValue
     */
    public function offsetGet(mixed $key): mixed {}

    /**
     * @param TKey|null $key
     * @param TValue $value
     */
    public function offsetSet(mixed $key, mixed $value): void {}

    /** @param TKey $key */
    public function offsetUnset(mixed $key): void {}

    /** @param TValue $value */
    public function append(mixed $value): void {}

    /** @return array<TKey, TValue> */
    public function getArrayCopy(): array {}

    public function count(): int {}

    public function getFlags(): int {}

    public function setFlags(int $flags): void {}

    public function asort(int $flags = SORT_REGULAR): bool {}

    public function ksort(int $flags = SORT_REGULAR): bool {}

    public function uasort(callable $callback): bool {}

    public function uksort(callable $callback): bool {}

    public function natsort(): bool {}

    public function natcasesort(): bool {}

    public function unserialize(string $data): void {}

    public function serialize(): string {}

    public function __serialize(): array {}

    public function __unserialize(array $data): void {}

    public function rewind(): void {}

    /** @return TValue */
    public function current(): mixed {}

    /** @return TKey */
    public function key(): string|int|null {}

    public function next(): void {}

    public function valid(): bool {}

    public function seek(int $offset): void {}

    public function __debugInfo(): array {}
}

/**
 * @template TKey of array-key
 * @template TValue
 * @implements \IteratorAggregate<TKey, TValue>
 * @implements \ArrayAccess<TKey, TValue>
 */
class ArrayObject implements \IteratorAggregate, \ArrayAccess, \Serializable, \Countable, \Traversable
{
    public const STD_PROP_LIST = 1;

    public const ARRAY_AS_PROPS = 2;

    /** @param array<TKey, TValue>|object $array */
    public function __construct(array|object $array = [], int $flags = 0, string $iteratorClass = \ArrayIterator::class) {}

    /** @param TKey $key */
    public function offsetExists(mixed $key): bool {}

    /**
     * @param TKey $key
     * @return TValue
     */
    public function offsetGet(mixed $key): mixed {}

    /**
     * @param TKey|null $key
     * @param TValue $value
     */
    public function offsetSet(mixed $key, mixed $value): void {}

    /** @param TKey $key */
    public function offsetUnset(mixed $key): void {}

    /** @param TValue $value */
    public function append(mixed $value): void {}

    /** @return array<TKey, TValue> */
    public function getArrayCopy(): array {}

    public function count(): int {}

    public function getFlags(): int {}

    public function setFlags(int $flags): void {}

    public function asort(int $flags = SORT_REGULAR): bool {}

    public function ksort(int $flags = SORT_REGULAR): bool {}

    public function uasort(callable $callback): bool {}

    public function uksort(callable $callback): bool {}

    public function natsort(): bool {}

    public function natcasesort(): bool {}

    public function unserialize(string $data): void {}

    public function serialize(): string {}

    public function __serialize(): array {}

    public function __unserialize(array $data): void {}

    /** @return \ArrayIterator<TKey, TValue> */
    public function getIterator(): \Iterator {}

    /** @return array<TKey, TValue> */
    public function exchangeArray(array|object $array): array {}

    public function setIteratorClass(string $iteratorClass): void {}

    public function getIteratorClass(): string {}

    public function __debugInfo(): array {}
}

/**
 * @template TObject of object
 * @template TData
 * @implements \Iterator<int, TObject>
 * @implements \ArrayAccess<TObject, TData>
 */
class SplObjectStorage implements \Countable, \Iterator, \Serializable, \ArrayAccess, \Traversable
{
    /**
     * @param TObject $object
     * @param TData $info
     */
    public function attach(object $object, mixed $info = null): void {}

    /** @param TObject $object */
    public function detach(object $object): void {}

    /** @param TObject $object */
    public function contains(object $object): bool {}

    /** @param \SplObjectStorage<TObject, TData> $storage */
    public function addAll(\SplObjectStorage $storage): int {}

    /** @param \SplObjectStorage<*, *> $storage */
    public function removeAll(\SplObjectStorage $storage): int {}

    /** @param \SplObjectStorage<*, *> $storage */
    public function removeAllExcept(\SplObjectStorage $storage): int {}

    /** @return TData */
    public function getInfo(): mixed {}

    /** @param TData $info */
    public function setInfo(mixed $info): void {}

    public function count(int $mode = COUNT_NORMAL): int {}

    public function rewind(): void {}

    public function valid(): bool {}

    public function key(): int {}

    /** @return TObject */
    public function current(): object {}

    public function next(): void {}

    public function unserialize(string $data): void {}

    public function serialize(): string {}

    /** @param TObject $object */
    public function offsetExists($object): bool {}

    /**
     * @param TObject $object
     * @return TData
     */
    public function offsetGet($object): mixed {}

    /**
     * @param TObject $object
     * @param TData $info
     */
    public function offsetSet($object, mixed $info = null): void {}

    /** @param TObject $object */
    public function offsetUnset($object): void {}

    /** @param TObject $object */
    public function getHash(object $object): string {}

    public function __serialize(): array {}

    public function __unserialize(array $data): void {}

    public function __debugInfo(): array {}
}

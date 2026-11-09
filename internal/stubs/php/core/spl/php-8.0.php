<?php

// PHP 8.0 delta: WeakMap, with its generic PHPDoc.

/**
 * @template TKey of object
 * @template TValue
 * @implements \IteratorAggregate<TKey, TValue>
 * @implements \ArrayAccess<TKey, TValue>
 */
final class WeakMap implements \ArrayAccess, \Countable, \IteratorAggregate, \Traversable
{
    /** @param TKey $object */
    public function offsetExists($object): bool {}

    /**
     * @param TKey $object
     * @return TValue
     */
    public function offsetGet($object): mixed {}

    /**
     * @param TKey $object
     * @param TValue $value
     */
    public function offsetSet($object, mixed $value): void {}

    /** @param TKey $object */
    public function offsetUnset($object): void {}

    public function count(): int {}

    /** @return \Iterator<TKey, TValue> */
    public function getIterator(): \Iterator {}
}

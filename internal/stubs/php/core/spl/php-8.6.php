<?php

// PHP 8.6 delta: members deprecated in 8.6, redeclared with #[\Deprecated].

class ArrayIterator
{
    #[\Deprecated(since: '8.6')]
    public function asort(int $flags = SORT_REGULAR): bool {}

    #[\Deprecated(since: '8.6')]
    public function getFlags(): int {}

    #[\Deprecated(since: '8.6')]
    public function ksort(int $flags = SORT_REGULAR): bool {}

    #[\Deprecated(since: '8.6')]
    public function natcasesort(): bool {}

    #[\Deprecated(since: '8.6')]
    public function natsort(): bool {}

    #[\Deprecated(since: '8.6')]
    public function serialize(): string {}

    #[\Deprecated(since: '8.6')]
    public function setFlags(int $flags): void {}

    #[\Deprecated(since: '8.6')]
    public function uasort(callable $callback): bool {}

    #[\Deprecated(since: '8.6')]
    public function uksort(callable $callback): bool {}

    #[\Deprecated(since: '8.6')]
    public function unserialize(string $data): void {}
}

<?php

// PHP 8.4 delta — only symbols not already in earlier baselines/deltas.
// Classes that already exist gain these members on top of their earlier ones.

/**
 * Marks a function, method or class constant as deprecated, so that using it
 * emits a deprecation notice.
 */
#[\Attribute(\Attribute::TARGET_METHOD | \Attribute::TARGET_FUNCTION | \Attribute::TARGET_CLASS_CONSTANT)]
final class Deprecated
{
    public readonly ?string $message;
    public readonly ?string $since;

    public function __construct(?string $message = null, ?string $since = null) {}
}

function array_find(array $array, callable $callback): mixed {}

function array_find_key(array $array, callable $callback): mixed {}

function array_any(array $array, callable $callback): bool {}

function array_all(array $array, callable $callback): bool {}


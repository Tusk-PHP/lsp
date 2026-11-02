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

class ReflectionClass
{
    public const SKIP_INITIALIZATION_ON_SERIALIZE = 8;
    public const SKIP_DESTRUCTOR = 2;

    /** @return T */
    public function newLazyGhost(callable $initializer, int $options = 0): object {}

    /** @return T */
    public function newLazyProxy(callable $factory, int $options = 0): object {}

    public function resetAsLazyGhost(object $object, callable $initializer, int $options = 0): void {}

    public function resetAsLazyProxy(object $object, callable $factory, int $options = 0): void {}

    public function initializeLazyObject(object $object): object {}

    public function isUninitializedLazyObject(object $object): bool {}

    public function markLazyObjectAsInitialized(object $object): object {}

    public function getLazyInitializer(object $object): ?callable {}
}

class ReflectionProperty
{
    public function setRawValueWithoutLazyInitialization(object $object, mixed $value): void {}

    public function skipLazyInitialization(object $object): void {}

    public function isLazy(object $object): bool {}

    public function getRawValue(object $object): mixed {}

    public function setRawValue(object $object, mixed $value): void {}

    public function isVirtual(): bool {}

    public function hasHooks(): bool {}

    public function getHooks(): array {}
}

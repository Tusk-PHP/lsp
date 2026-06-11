<?php

// PHP 8.1 delta — only symbols not already in the 7.4 baseline or 8.0 delta.

/**
 * Checks whether a given value is a list array (sequential integer keys
 * starting from 0).
 *
 * @param mixed $array
 */
function array_is_list(mixed $array): bool {}

/**
 * Checks whether the given name is the name of an existing enum.
 */
function enum_exists(string $enum, bool $autoload = true): bool {}

/**
 * Returns the current memory usage peak in bytes and optionally resets the
 * real-usage peak tracker.
 */
function memory_reset_peak_usage(): void {}

interface UnitEnum
{
    public readonly string $name;

    /** @return static[] */
    public static function cases(): array;
}

interface BackedEnum extends UnitEnum
{
    public readonly int|string $value;

    public static function from(int|string $value): static;

    public static function tryFrom(int|string $value): ?static;
}

/** @template T of \UnitEnum */
class ReflectionEnum extends \ReflectionClass
{
    /** @param class-string<T>|T $objectOrClass */
    public function __construct(object|string $objectOrClass) {}

    public function hasCase(string $name): bool {}

    public function getCase(string $name): ReflectionEnumUnitCase {}

    /** @return ReflectionEnumUnitCase[] */
    public function getCases(): array {}

    public function isBacked(): bool {}

    public function getBackingType(): ?\ReflectionNamedType {}
}

class ReflectionEnumUnitCase extends \ReflectionClassConstant
{
    public function getEnum(): ReflectionEnum {}

    public function getValue(): \UnitEnum {}
}

class ReflectionEnumBackedCase extends ReflectionEnumUnitCase
{
    public function getBackingValue(): int|string {}
}

class ReflectionFiber
{
    public function __construct(\Fiber $fiber) {}

    public function getFiber(): \Fiber {}

    public function getExecutingFile(): ?string {}

    public function getExecutingLine(): ?int {}

    public function getCallable(): callable {}

    public function isStarted(): bool {}

    public function isRunning(): bool {}

    public function isSuspended(): bool {}

    public function isTerminated(): bool {}

    public function getReturn(): mixed {}
}

class Fiber
{
    public function __construct(callable $callback) {}

    public function start(mixed ...$args): mixed {}

    public function resume(mixed $value = null): mixed {}

    public function getReturn(): mixed {}

    public function isStarted(): bool {}

    public function isRunning(): bool {}

    public function isSuspended(): bool {}

    public function isTerminated(): bool {}

    public static function this(): ?static {}

    public static function suspend(mixed $value = null): mixed {}
}

/** Attribute class that marks a class as allowing dynamic properties (PHP 8.1). */
#[\Attribute(\Attribute::TARGET_CLASS)]
final class AllowDynamicProperties {}

/** Attribute to mark a function or method return type as changed. */
#[\Attribute(\Attribute::TARGET_METHOD)]
final class ReturnTypeWillChange {}

class CURLStringFile
{
    public string $data;
    public string $postname;
    public string $mime;

    public function __construct(string $data, string $postname, string $mime = 'application/octet-stream') {}
}

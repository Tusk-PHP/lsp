<?php

// PHP 8.4 delta: members added in 8.4, merged into the classes declared earlier.

final class ReflectionGenerator
{
    public function isClosed(): bool {}
}

class ReflectionClass
{
    public const SKIP_DESTRUCTOR = 16;

    public const SKIP_INITIALIZATION_ON_SERIALIZE = 8;

    public function getLazyInitializer(object $object): ?callable {}

    public function initializeLazyObject(object $object): object {}

    public function isUninitializedLazyObject(object $object): bool {}

    public function markLazyObjectAsInitialized(object $object): object {}

    /** @return T */
    public function newLazyGhost(callable $initializer, int $options = 0): object {}

    /** @return T */
    public function newLazyProxy(callable $factory, int $options = 0): object {}

    public function resetAsLazyGhost(object $object, callable $initializer, int $options = 0): void {}

    public function resetAsLazyProxy(object $object, callable $factory, int $options = 0): void {}
}

class ReflectionProperty
{
    public const IS_ABSTRACT = 64;

    public const IS_FINAL = 32;

    public const IS_PRIVATE_SET = 4096;

    public const IS_PROTECTED_SET = 2048;

    public const IS_VIRTUAL = 512;

    public function getHook(\PropertyHookType $type): ?\ReflectionMethod {}

    public function getHooks(): array {}

    public function getRawValue(object $object): mixed {}

    public function getSettableType(): ?\ReflectionType {}

    public function hasHook(\PropertyHookType $type): bool {}

    public function hasHooks(): bool {}

    public function isAbstract(): bool {}

    public function isDynamic(): bool {}

    public function isFinal(): bool {}

    public function isLazy(object $object): bool {}

    public function isPrivateSet(): bool {}

    public function isProtectedSet(): bool {}

    public function isVirtual(): bool {}

    public function setRawValue(object $object, mixed $value): void {}

    public function setRawValueWithoutLazyInitialization(object $object, mixed $value): void {}

    public function skipLazyInitialization(object $object): void {}
}

class ReflectionClassConstant
{
    public function isDeprecated(): bool {}
}

class ReflectionConstant implements \Reflector
{
    public string $name;

    public function __construct(string $name) {}

    public function __toString(): string {}

    public function getName(): string {}

    public function getNamespaceName(): string {}

    public function getShortName(): string {}

    public function getValue(): mixed {}

    public function isDeprecated(): bool {}
}

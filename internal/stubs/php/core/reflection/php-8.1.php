<?php

// PHP 8.1 delta: members added in 8.1, merged into the classes declared earlier.

abstract class ReflectionFunctionAbstract
{
    public function getClosureUsedVariables(): array {}

    public function getTentativeReturnType(): ?\ReflectionType {}

    public function hasTentativeReturnType(): bool {}

    public function isStatic(): bool {}
}

class ReflectionClass
{
    public function isEnum(): bool {}
}

class ReflectionProperty
{
    public const IS_READONLY = 128;

    public function isReadOnly(): bool {}
}

class ReflectionClassConstant
{
    public const IS_FINAL = 32;

    public function isEnumCase(): bool {}

    public function isFinal(): bool {}
}

class ReflectionIntersectionType extends \ReflectionType
{
    /** @return ReflectionNamedType[] */
    public function getTypes(): array {}
}

/** @template T of \UnitEnum */
class ReflectionEnum extends \ReflectionClass
{
    /** @param class-string<T>|T $objectOrClass */
    public function __construct(object|string $objectOrClass) {}

    public function getBackingType(): ?\ReflectionNamedType {}

    public function getCase(string $name): \ReflectionEnumUnitCase {}

    /** @return ReflectionEnumUnitCase[] */
    public function getCases(): array {}

    public function hasCase(string $name): bool {}

    public function isBacked(): bool {}
}

class ReflectionEnumUnitCase extends \ReflectionClassConstant
{
    public function __construct(object|string $class, string $constant) {}

    public function getEnum(): \ReflectionEnum {}

    public function getValue(): \UnitEnum {}
}

class ReflectionEnumBackedCase extends \ReflectionEnumUnitCase
{
    public function __construct(object|string $class, string $constant) {}

    public function getBackingValue(): string|int {}
}

final class ReflectionFiber
{
    public function __construct(\Fiber $fiber) {}

    public function getCallable(): callable {}

    public function getExecutingFile(): ?string {}

    public function getExecutingLine(): ?int {}

    public function getFiber(): \Fiber {}

    public function getTrace(int $options = DEBUG_BACKTRACE_PROVIDE_OBJECT): array {}
}

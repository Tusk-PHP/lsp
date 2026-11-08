<?php

// PHP 8.0 delta: members added in 8.0, merged into the classes declared earlier.

abstract class ReflectionFunctionAbstract
{
    /** @return ReflectionAttribute[] */
    public function getAttributes(?string $name = null, int $flags = 0): array {}

    public function getClosureCalledClass(): ?\ReflectionClass {}
}

class ReflectionClass
{
    /** @return ReflectionAttribute[] */
    public function getAttributes(?string $name = null, int $flags = 0): array {}
}

class ReflectionProperty
{
    /** @return ReflectionAttribute[] */
    public function getAttributes(?string $name = null, int $flags = 0): array {}

    public function getDefaultValue(): mixed {}

    public function hasDefaultValue(): bool {}

    public function isPromoted(): bool {}
}

class ReflectionClassConstant
{
    public const IS_PRIVATE = 4;

    public const IS_PROTECTED = 2;

    public const IS_PUBLIC = 1;

    /** @return ReflectionAttribute[] */
    public function getAttributes(?string $name = null, int $flags = 0): array {}
}

class ReflectionParameter
{
    /** @return ReflectionAttribute[] */
    public function getAttributes(?string $name = null, int $flags = 0): array {}

    public function isPromoted(): bool {}
}

class ReflectionUnionType extends \ReflectionType
{
    /** @return ReflectionNamedType[] */
    public function getTypes(): array {}
}

class ReflectionAttribute implements \Reflector
{
    public const IS_INSTANCEOF = 2;

    public string $name;

    private function __clone(): void {}

    private function __construct() {}

    public function __toString(): string {}

    public function getArguments(): array {}

    public function getName(): string {}

    public function getTarget(): int {}

    public function isRepeated(): bool {}

    public function newInstance(): object {}
}

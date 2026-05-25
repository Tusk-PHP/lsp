<?php

class ReflectionMethod
{
    public const IS_STATIC = 16;
    public const IS_PUBLIC = 1;

    public function __construct(object|string $objectOrMethod, string $method = '') {}

    public function getName(): string {}

    public function getDeclaringClass(): ReflectionClass {}

    public function getParameters(): array {}

    public function invoke(object $object, mixed ...$args): mixed {}

    public function isConstructor(): bool {}

    public function isPublic(): bool {}

    public function isStatic(): bool {}
}

class ReflectionClass
{
    public function __construct(object|string $objectOrClass) {}

    public function getName(): string {}
}

class ReflectionObject
{
    public function __construct(object $object) {}

    public function getName(): string {}

    public function getMethod(string $name): ReflectionMethod {}

    public function getMethods(int $filter = 0): array {}

    public function hasMethod(string $name): bool {}
}

class ReflectionParameter
{
    public function getName(): string {}

    public function isDefaultValueAvailable(): bool {}

    public function getDefaultValue(): mixed {}
}

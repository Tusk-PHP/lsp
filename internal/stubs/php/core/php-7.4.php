<?php

interface Throwable
{
    public function getMessage(): string;
    public function getCode(): int;
}

class Exception implements Throwable
{
    public function __construct(string $message = '', int $code = 0) {}

    public function getMessage(): string {}

    public function getCode(): int {}
}

class InvalidArgumentException extends Exception
{
}

interface DateTimeInterface
{
    public function format(string $format): string;

    public function getTimestamp(): int;
}

class DateTime implements DateTimeInterface
{
    public function __construct(string $datetime = 'now') {}

    public function format(string $format): string {}

    public function getTimestamp(): int {}
}

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

function array_reverse(array $array, bool $preserve_keys = false): array {}

function var_export(mixed $value, bool $return = false): string {}

function implode(string $separator, array $array): string {}

function get_class(object $object): string {}

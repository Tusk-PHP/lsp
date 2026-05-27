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

abstract class ReflectionFunctionAbstract
{
    public string $name;

    public function getName(): string {}

    public function getDocComment(): string|false {}

    public function getEndLine(): int|false {}

    public function getFileName(): string|false {}

    public function getNamespaceName(): string {}

    public function getNumberOfParameters(): int {}

    public function getNumberOfRequiredParameters(): int {}

    /** @return ReflectionParameter[] */
    public function getParameters(): array {}

    public function getReturnType(): ?ReflectionType {}

    public function getShortName(): string {}

    public function getStartLine(): int|false {}

    public function hasReturnType(): bool {}

    public function inNamespace(): bool {}

    public function isDeprecated(): bool {}

    public function isInternal(): bool {}

    public function isUserDefined(): bool {}

    public function isVariadic(): bool {}

    public function returnsReference(): bool {}
}

class ReflectionMethod extends ReflectionFunctionAbstract
{
    public string $class;

    public const IS_STATIC = 16;
    public const IS_PUBLIC = 1;
    public const IS_PROTECTED = 2;
    public const IS_PRIVATE = 4;
    public const IS_ABSTRACT = 64;
    public const IS_FINAL = 32;

    public function __construct(object|string $objectOrMethod, string $method = '') {}

    public function getDeclaringClass(): ReflectionClass {}

    public function getModifiers(): int {}

    public function getPrototype(): ReflectionMethod {}

    public function invoke(object $object, mixed ...$args): mixed {}

    public function invokeArgs(?object $object, array $args): mixed {}

    public function isAbstract(): bool {}

    public function isConstructor(): bool {}

    public function isDestructor(): bool {}

    public function isFinal(): bool {}

    public function isPrivate(): bool {}

    public function isProtected(): bool {}

    public function isPublic(): bool {}

    public function isStatic(): bool {}
}

class ReflectionType
{
    public function allowsNull(): bool {}

    public function __toString(): string {}
}

/** @template T of object */
class ReflectionClass
{
    /** @param class-string<T>|T $objectOrClass */
    public function __construct(object|string $objectOrClass) {}

    /** @return class-string<T> */
    public function getName(): string {}

    /** @return T */
    public function newInstance(mixed ...$args): object {}

    /** @return T */
    public function newInstanceArgs(array $args = []): object {}

    /** @return T */
    public function newInstanceWithoutConstructor(): object {}

    public function getMethod(string $name): ReflectionMethod {}

    /** @return ReflectionMethod[] */
    public function getMethods(int $filter = 0): array {}

    public function hasMethod(string $name): bool {}
}

class ReflectionObject extends ReflectionClass
{
    public function __construct(object $object) {}

    public function getMethod(string $name): ReflectionMethod {}

    /** @return ReflectionMethod[] */
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

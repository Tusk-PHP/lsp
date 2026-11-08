<?php

// PHP 7.4 baseline. Signatures from PHP's own reflection; members added in later
// versions are in the matching php-X.Y.php delta.

interface Reflector
{

}

class Reflection
{
    public static function getModifierNames(int $modifiers): array {}
}

class ReflectionException extends \Exception
{

}

abstract class ReflectionFunctionAbstract implements \Reflector
{
    public string $name;

    private function __clone(): void {}

    public function getClosureScopeClass(): ?\ReflectionClass {}

    public function getClosureThis(): ?object {}

    public function getDocComment(): string|false {}

    public function getEndLine(): int|false {}

    public function getExtension(): ?\ReflectionExtension {}

    public function getExtensionName(): string|false {}

    public function getFileName(): string|false {}

    public function getName(): string {}

    public function getNamespaceName(): string {}

    public function getNumberOfParameters(): int {}

    public function getNumberOfRequiredParameters(): int {}

    /** @return ReflectionParameter[] */
    public function getParameters(): array {}

    public function getReturnType(): ?\ReflectionType {}

    public function getShortName(): string {}

    public function getStartLine(): int|false {}

    public function getStaticVariables(): array {}

    public function hasReturnType(): bool {}

    public function inNamespace(): bool {}

    public function isClosure(): bool {}

    public function isDeprecated(): bool {}

    public function isGenerator(): bool {}

    public function isInternal(): bool {}

    public function isUserDefined(): bool {}

    public function isVariadic(): bool {}

    public function returnsReference(): bool {}
}

class ReflectionFunction extends \ReflectionFunctionAbstract
{
    public const IS_DEPRECATED = 2048;

    public function __construct(\Closure|string $function) {}

    public function __toString(): string {}

    public function getClosure(): \Closure {}

    public function invoke(mixed ...$args): mixed {}

    public function invokeArgs(array $args): mixed {}

    public function isDisabled(): bool {}
}

final class ReflectionGenerator
{
    public function __construct(\Generator $generator) {}

    public function getExecutingFile(): string {}

    public function getExecutingGenerator(): \Generator {}

    public function getExecutingLine(): int {}

    public function getFunction(): \ReflectionFunctionAbstract {}

    public function getThis(): ?object {}

    public function getTrace(int $options = DEBUG_BACKTRACE_PROVIDE_OBJECT): array {}
}

class ReflectionMethod extends \ReflectionFunctionAbstract
{
    public const IS_ABSTRACT = 64;

    public const IS_FINAL = 32;

    public const IS_PRIVATE = 4;

    public const IS_PROTECTED = 2;

    public const IS_PUBLIC = 1;

    public const IS_STATIC = 16;

    public string $class;

    public function __construct(object|string $objectOrMethod, ?string $method = null) {}

    public function __toString(): string {}

    public function getClosure(?object $object = null): \Closure {}

    public function getDeclaringClass(): \ReflectionClass {}

    public function getModifiers(): int {}

    public function getPrototype(): \ReflectionMethod {}

    public function invoke(?object $object, mixed ...$args): mixed {}

    public function invokeArgs(?object $object, array $args): mixed {}

    public function isAbstract(): bool {}

    public function isConstructor(): bool {}

    public function isDestructor(): bool {}

    public function isFinal(): bool {}

    public function isPrivate(): bool {}

    public function isProtected(): bool {}

    public function isPublic(): bool {}

    public function setAccessible(bool $accessible): void {}
}

/** @template T of object */
class ReflectionClass implements \Reflector
{
    public const IS_EXPLICIT_ABSTRACT = 64;

    public const IS_FINAL = 32;

    public const IS_IMPLICIT_ABSTRACT = 16;

    public string $name;

    private function __clone(): void {}

    /** @param class-string<T>|T $objectOrClass */
    public function __construct(object|string $objectOrClass) {}

    public function __toString(): string {}

    public function getConstant(string $name): mixed {}

    public function getConstants(?int $filter = null): array {}

    public function getConstructor(): ?\ReflectionMethod {}

    public function getDefaultProperties(): array {}

    public function getDocComment(): string|false {}

    public function getEndLine(): int|false {}

    public function getExtension(): ?\ReflectionExtension {}

    public function getExtensionName(): string|false {}

    public function getFileName(): string|false {}

    public function getInterfaceNames(): array {}

    /** @return array<string, ReflectionClass> */
    public function getInterfaces(): array {}

    public function getMethod(string $name): \ReflectionMethod {}

    /** @return ReflectionMethod[] */
    public function getMethods(?int $filter = null): array {}

    public function getModifiers(): int {}

    /** @return class-string<T> */
    public function getName(): string {}

    public function getNamespaceName(): string {}

    public function getParentClass(): \ReflectionClass|false {}

    /** @return ReflectionProperty[] */
    public function getProperties(?int $filter = null): array {}

    public function getProperty(string $name): \ReflectionProperty {}

    public function getReflectionConstant(string $name): \ReflectionClassConstant|false {}

    /** @return ReflectionClassConstant[] */
    public function getReflectionConstants(?int $filter = null): array {}

    public function getShortName(): string {}

    public function getStartLine(): int|false {}

    public function getStaticProperties(): array {}

    public function getStaticPropertyValue(string $name, mixed $default = null): mixed {}

    public function getTraitAliases(): array {}

    public function getTraitNames(): array {}

    /** @return array<string, ReflectionClass> */
    public function getTraits(): array {}

    public function hasConstant(string $name): bool {}

    public function hasMethod(string $name): bool {}

    public function hasProperty(string $name): bool {}

    public function implementsInterface(\ReflectionClass|string $interface): bool {}

    public function inNamespace(): bool {}

    public function isAbstract(): bool {}

    public function isAnonymous(): bool {}

    public function isCloneable(): bool {}

    public function isFinal(): bool {}

    public function isInstance(object $object): bool {}

    public function isInstantiable(): bool {}

    public function isInterface(): bool {}

    public function isInternal(): bool {}

    public function isIterable(): bool {}

    public function isIterateable(): bool {}

    public function isSubclassOf(\ReflectionClass|string $class): bool {}

    public function isTrait(): bool {}

    public function isUserDefined(): bool {}

    /** @return T */
    public function newInstance(mixed ...$args): object {}

    /** @return T|null */
    public function newInstanceArgs(array $args = []): ?object {}

    /** @return T */
    public function newInstanceWithoutConstructor(): object {}

    public function setStaticPropertyValue(string $name, mixed $value): void {}
}

class ReflectionObject extends \ReflectionClass
{
    public function __construct(object $object) {}
}

class ReflectionProperty implements \Reflector
{
    public const IS_PRIVATE = 4;

    public const IS_PROTECTED = 2;

    public const IS_PUBLIC = 1;

    public const IS_STATIC = 16;

    public string $class;

    public string $name;

    private function __clone(): void {}

    public function __construct(object|string $class, string $property) {}

    public function __toString(): string {}

    public function getDeclaringClass(): \ReflectionClass {}

    public function getDocComment(): string|false {}

    public function getModifiers(): int {}

    public function getName(): string {}

    public function getType(): ?\ReflectionType {}

    public function getValue(?object $object = null): mixed {}

    public function hasType(): bool {}

    public function isDefault(): bool {}

    public function isInitialized(?object $object = null): bool {}

    public function isPrivate(): bool {}

    public function isProtected(): bool {}

    public function isPublic(): bool {}

    public function isStatic(): bool {}

    public function setAccessible(bool $accessible): void {}

    public function setValue(mixed $objectOrValue, mixed $value = null): void {}
}

class ReflectionClassConstant implements \Reflector
{
    public string $class;

    public string $name;

    private function __clone(): void {}

    public function __construct(object|string $class, string $constant) {}

    public function __toString(): string {}

    public function getDeclaringClass(): \ReflectionClass {}

    public function getDocComment(): string|false {}

    public function getModifiers(): int {}

    public function getName(): string {}

    public function getValue(): mixed {}

    public function isPrivate(): bool {}

    public function isProtected(): bool {}

    public function isPublic(): bool {}
}

class ReflectionParameter implements \Reflector
{
    public string $name;

    private function __clone(): void {}

    public function __construct($function, string|int $param) {}

    public function __toString(): string {}

    public function allowsNull(): bool {}

    public function canBePassedByValue(): bool {}

    public function getClass(): ?\ReflectionClass {}

    public function getDeclaringClass(): ?\ReflectionClass {}

    public function getDeclaringFunction(): \ReflectionFunctionAbstract {}

    public function getDefaultValue(): mixed {}

    public function getDefaultValueConstantName(): ?string {}

    public function getName(): string {}

    public function getPosition(): int {}

    public function getType(): ?\ReflectionType {}

    public function hasType(): bool {}

    public function isArray(): bool {}

    public function isCallable(): bool {}

    public function isDefaultValueAvailable(): bool {}

    public function isDefaultValueConstant(): bool {}

    public function isOptional(): bool {}

    public function isPassedByReference(): bool {}

    public function isVariadic(): bool {}
}

abstract class ReflectionType
{
    private function __clone(): void {}

    public function __toString(): string {}

    public function allowsNull(): bool {}
}

class ReflectionNamedType extends \ReflectionType
{
    public function getName(): string {}

    public function isBuiltin(): bool {}
}

final class ReflectionReference
{
    private function __clone(): void {}

    private function __construct() {}

    public static function fromArrayElement(array $array, string|int $key): ?\ReflectionReference {}

    public function getId(): string {}
}

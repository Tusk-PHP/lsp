<?php

// PHP 8.6 delta: members added in 8.6, merged into the classes declared earlier.

class ReflectionAttribute
{
    public function getNamespaceName(): string {}

    public function getShortName(): string {}

    public function inNamespace(): bool {}
}

class ReflectionConstant
{
    public function inNamespace(): bool {}
}

class ReflectionParameter
{
    public function getDocComment(): string|false {}
}

class ReflectionProperty
{
    public function isReadable(?string $scope, ?object $object = NULL): bool {}

    public function isWritable(?string $scope, ?object $object = NULL): bool {}
}

<?php

// PHP 8.2 delta: members added in 8.2, merged into the classes declared earlier.

class ReflectionFunction
{
    public function isAnonymous(): bool {}
}

class ReflectionMethod
{
    public function hasPrototype(): bool {}
}

class ReflectionClass
{
    public const IS_READONLY = 65536;

    public function isReadOnly(): bool {}
}

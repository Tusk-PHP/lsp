<?php

// PHP 8.3 delta: members added in 8.3, merged into the classes declared earlier.

class ReflectionMethod
{
    public static function createFromMethodName(string $method): static {}
}

class ReflectionClassConstant
{
    public function getType(): ?\ReflectionType {}

    public function hasType(): bool {}
}

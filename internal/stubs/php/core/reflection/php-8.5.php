<?php

// PHP 8.5 delta: members added in 8.5, merged into the classes declared earlier.

class ReflectionProperty
{
    public function getMangledName(): string {}
}

class ReflectionConstant
{
    /** @return ReflectionAttribute[] */
    public function getAttributes(?string $name = null, int $flags = 0): array {}

    public function getExtension(): ?\ReflectionExtension {}

    public function getExtensionName(): string|false {}

    public function getFileName(): string|false {}
}

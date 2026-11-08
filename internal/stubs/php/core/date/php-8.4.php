<?php

// PHP 8.4 delta: members added in 8.4, merged into the classes declared earlier.

class DateTime
{
    public static function createFromTimestamp(int|float $timestamp): static {}

    public function getMicrosecond(): int {}

    public function setMicrosecond(int $microsecond): static {}
}

class DateTimeImmutable
{
    public static function createFromTimestamp(int|float $timestamp): static {}

    public function getMicrosecond(): int {}

    public function setMicrosecond(int $microsecond): static {}
}

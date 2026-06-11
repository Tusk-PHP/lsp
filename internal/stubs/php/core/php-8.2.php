<?php

// PHP 8.2 delta — only symbols not already in earlier baselines/deltas.

/**
 * Parses a human-readable byte size string such as "256M" or "1G" into bytes.
 * Recognises the same suffixes as php.ini (K, M, G, T, P and E).
 */
function ini_parse_quantity(string $shorthand): int {}

/**
 * Attribute that marks a parameter as containing sensitive data so that it is
 * redacted in stack traces (PHP 8.2+).
 */
#[\Attribute(\Attribute::TARGET_PARAMETER)]
final class SensitiveParameter {}

class SensitiveParameterValue
{
    public function __construct(mixed $value) {}

    public function getValue(): mixed {}
}

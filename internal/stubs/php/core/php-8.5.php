<?php

// PHP 8.5 delta — only symbols not already in earlier baselines/deltas.

/**
 * Marks a function or method whose return value must be used, so that
 * discarding it emits a warning. Cast to (void) to discard it on purpose.
 */
#[\Attribute(\Attribute::TARGET_METHOD | \Attribute::TARGET_FUNCTION)]
final class NoDiscard
{
    public readonly ?string $message;

    public function __construct(?string $message = null) {}
}

function array_first(array $array): mixed {}

function array_last(array $array): mixed {}

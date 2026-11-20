<?php

// PHP 8.6 delta: Time functions, classes and enums added in 8.6,
// rendered from PHP 8.6.0's reflection. Classes list all their members.

namespace Time;

/**
 * An immutable length of time with nanosecond precision, made with the
 * static from*() constructors and combined with add(), sub(), multiplyBy()
 * and divideBy().
 */
final readonly class Duration {
    public readonly int $nanoseconds;
    public readonly bool $negative;
    public readonly int $seconds;
    private function __construct() {}
    public function __unserialize(array $data): void {}
    public function absolute(): \Time\Duration {}
    public function add(\Time\Duration $duration): \Time\Duration {}
    public static function compare(\Time\Duration $a, \Time\Duration $b): int {}
    public function divideBy(int $divisor): \Time\Duration {}
    public static function fromHours(int $hours): \Time\Duration {}
    public static function fromIso8601DurationString(string $specification): \Time\Duration {}
    public static function fromMicroseconds(int $microseconds): \Time\Duration {}
    public static function fromMilliseconds(int $milliseconds): \Time\Duration {}
    public static function fromMinutes(int $minutes): \Time\Duration {}
    public static function fromNanoseconds(int $nanoseconds): \Time\Duration {}
    public static function fromSeconds(int $seconds, int $nanoseconds = 0): \Time\Duration {}
    public function multiplyBy(int $factor): \Time\Duration {}
    public function negate(): \Time\Duration {}
    public function sub(\Time\Duration $duration): \Time\Duration {}
}

class TimeException extends \Exception implements \Stringable, \Throwable {
}

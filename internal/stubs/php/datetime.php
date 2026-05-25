<?php

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

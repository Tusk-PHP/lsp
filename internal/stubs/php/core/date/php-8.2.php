<?php

// PHP 8.2 delta: members added in 8.2, merged into the classes declared earlier.

class DateTime
{
    public function __serialize(): array {}

    public function __unserialize(array $data): void {}
}

class DateTimeImmutable
{
    public function __serialize(): array {}

    public function __unserialize(array $data): void {}
}

class DateTimeZone
{
    public function __serialize(): array {}

    public function __unserialize(array $data): void {}
}

class DateInterval
{
    public bool $from_string;

    public function __serialize(): array {}

    public function __unserialize(array $data): void {}
}

class DatePeriod
{
    public const INCLUDE_END_DATE = 2;

    public bool $include_end_date;

    public function __serialize(): array {}

    public function __unserialize(array $data): void {}
}

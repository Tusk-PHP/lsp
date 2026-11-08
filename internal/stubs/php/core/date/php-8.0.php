<?php

// PHP 8.0 delta: members added in 8.0, merged into the classes declared earlier.

class DateTime
{
    public static function createFromInterface(\DateTimeInterface $object): \DateTime {}
}

class DateTimeImmutable
{
    public static function createFromInterface(\DateTimeInterface $object): \DateTimeImmutable {}
}

class DatePeriod
{
    public function getIterator(): \Iterator {}
}

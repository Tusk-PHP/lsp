<?php

// PHP 7.4 baseline. Signatures from PHP's own reflection; members added in later
// versions are in the matching php-X.Y.php delta.

interface DateTimeInterface
{
    const ATOM = 'Y-m-d\\TH:i:sP';

    const COOKIE = 'l, d-M-Y H:i:s T';

    const ISO8601 = 'Y-m-d\\TH:i:sO';

    const RFC1036 = 'D, d M y H:i:s O';

    const RFC1123 = 'D, d M Y H:i:s O';

    const RFC2822 = 'D, d M Y H:i:s O';

    const RFC3339 = 'Y-m-d\\TH:i:sP';

    const RFC3339_EXTENDED = 'Y-m-d\\TH:i:s.vP';

    const RFC7231 = 'D, d M Y H:i:s \\G\\M\\T';

    const RFC822 = 'D, d M y H:i:s O';

    const RFC850 = 'l, d-M-y H:i:s T';

    const RSS = 'D, d M Y H:i:s O';

    const W3C = 'Y-m-d\\TH:i:sP';

    public function __wakeup(): void;

    public function diff(\DateTimeInterface $targetObject, bool $absolute = false): \DateInterval;

    public function format(string $format): string;

    public function getOffset(): int;

    public function getTimestamp(): int;

    public function getTimezone(): \DateTimeZone|false;

}

class DateTime implements \DateTimeInterface
{
    public function __construct(string $datetime = 'now', ?\DateTimeZone $timezone = null) {}

    public static function __set_state(array $array): \DateTime {}

    public function __wakeup(): void {}

    public function add(\DateInterval $interval): \DateTime {}

    public static function createFromFormat(string $format, string $datetime, ?\DateTimeZone $timezone = null): \DateTime|false {}

    public static function createFromImmutable(\DateTimeImmutable $object): static {}

    public function diff(\DateTimeInterface $targetObject, bool $absolute = false): \DateInterval {}

    public function format(string $format): string {}

    public static function getLastErrors(): array|false {}

    public function getOffset(): int {}

    public function getTimestamp(): int {}

    public function getTimezone(): \DateTimeZone|false {}

    public function modify(string $modifier): \DateTime {}

    public function setDate(int $year, int $month, int $day): \DateTime {}

    public function setISODate(int $year, int $week, int $dayOfWeek = 1): \DateTime {}

    public function setTime(int $hour, int $minute, int $second = 0, int $microsecond = 0): \DateTime {}

    public function setTimestamp(int $timestamp): \DateTime {}

    public function setTimezone(\DateTimeZone $timezone): \DateTime {}

    public function sub(\DateInterval $interval): \DateTime {}
}

class DateTimeImmutable implements \DateTimeInterface
{
    public function __construct(string $datetime = 'now', ?\DateTimeZone $timezone = null) {}

    public static function __set_state(array $array): \DateTimeImmutable {}

    public function __wakeup(): void {}

    public function add(\DateInterval $interval): \DateTimeImmutable {}

    public static function createFromFormat(string $format, string $datetime, ?\DateTimeZone $timezone = null): \DateTimeImmutable|false {}

    public static function createFromMutable(\DateTime $object): static {}

    public function diff(\DateTimeInterface $targetObject, bool $absolute = false): \DateInterval {}

    public function format(string $format): string {}

    public static function getLastErrors(): array|false {}

    public function getOffset(): int {}

    public function getTimestamp(): int {}

    public function getTimezone(): \DateTimeZone|false {}

    public function modify(string $modifier): \DateTimeImmutable {}

    public function setDate(int $year, int $month, int $day): \DateTimeImmutable {}

    public function setISODate(int $year, int $week, int $dayOfWeek = 1): \DateTimeImmutable {}

    public function setTime(int $hour, int $minute, int $second = 0, int $microsecond = 0): \DateTimeImmutable {}

    public function setTimestamp(int $timestamp): \DateTimeImmutable {}

    public function setTimezone(\DateTimeZone $timezone): \DateTimeImmutable {}

    public function sub(\DateInterval $interval): \DateTimeImmutable {}
}

class DateTimeZone
{
    public const AFRICA = 1;

    public const ALL = 2047;

    public const ALL_WITH_BC = 4095;

    public const AMERICA = 2;

    public const ANTARCTICA = 4;

    public const ARCTIC = 8;

    public const ASIA = 16;

    public const ATLANTIC = 32;

    public const AUSTRALIA = 64;

    public const EUROPE = 128;

    public const INDIAN = 256;

    public const PACIFIC = 512;

    public const PER_COUNTRY = 4096;

    public const UTC = 1024;

    public function __construct(string $timezone) {}

    public static function __set_state(array $array): \DateTimeZone {}

    public function __wakeup(): void {}

    public function getLocation(): array|false {}

    public function getName(): string {}

    public function getOffset(\DateTimeInterface $datetime): int {}

    public function getTransitions(int $timestampBegin = PHP_INT_MIN, int $timestampEnd = 2147483647): array|false {}

    public static function listAbbreviations(): array {}

    public static function listIdentifiers(int $timezoneGroup = DateTimeZone::ALL, ?string $countryCode = null): array {}
}

class DateInterval
{
    public int $y;

    public int $m;

    public int $d;

    public int $h;

    public int $i;

    public int $s;

    public float $f;

    public int $invert;

    /** @var int|false */
    public $days;

    public function __construct(string $duration) {}

    public static function __set_state(array $array): \DateInterval {}

    public function __wakeup(): void {}

    public static function createFromDateString(string $datetime): \DateInterval {}

    public function format(string $format): string {}
}

class DatePeriod implements \IteratorAggregate
{
    public const EXCLUDE_START_DATE = 1;

    public ?\DateTimeInterface $current;

    public ?\DateTimeInterface $end;

    public bool $include_start_date;

    public ?\DateInterval $interval;

    public int $recurrences;

    public ?\DateTimeInterface $start;

    public function __construct($start, $interval = null, $end = null, $options = null) {}

    public static function __set_state(array $array): \DatePeriod {}

    public function __wakeup(): void {}

    public function getDateInterval(): \DateInterval {}

    public function getEndDate(): ?\DateTimeInterface {}

    public function getRecurrences(): ?int {}

    public function getStartDate(): \DateTimeInterface {}
}

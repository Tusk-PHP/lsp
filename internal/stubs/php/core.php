<?php

interface Throwable
{
    public function getMessage(): string;
    public function getCode(): int;
}

class Exception implements Throwable
{
    public function __construct(string $message = '', int $code = 0) {}

    public function getMessage(): string {}

    public function getCode(): int {}
}

class InvalidArgumentException extends Exception
{
}

function array_reverse(array $array, bool $preserve_keys = false): array {}

function var_export(mixed $value, bool $return = false): string {}

function implode(string $separator, array $array): string {}

function get_class(object $object): string {}

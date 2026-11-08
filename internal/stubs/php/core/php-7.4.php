<?php

function array_reverse(array $array, bool $preserve_keys = false): array {}

function var_export(mixed $value, bool $return = false): string {}

function implode(string $separator, array $array): string {}

function get_class(object $object): string {}

// Windows-only functions, missing from a builtin list generated on another OS.

/** @param resource $stream */
function sapi_windows_vt100_support($stream, ?bool $enable = null): bool {}

function sapi_windows_cp_set(int $codepage): bool {}

function sapi_windows_cp_get(string $kind = ''): int {}

function sapi_windows_cp_is_utf8(): bool {}

function sapi_windows_cp_conv(int|string $in_codepage, int|string $out_codepage, string $subject): ?string {}

function sapi_windows_set_ctrl_handler(?callable $handler, bool $add = true): bool {}

function sapi_windows_generate_ctrl_event(int $event, int $pid = 0): bool {}

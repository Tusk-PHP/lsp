<?php

function json_encode(mixed $value): string|false {}

function json_decode(string $json, ?bool $associative = null): mixed {}

function json_last_error(): int {}

function json_last_error_msg(): string {}

<?php

// PHP 8.6 delta: Openssl functions, classes and enums added in 8.6,
// rendered from PHP 8.6.0's reflection. Classes list all their members.

namespace Openssl;

class OpensslException extends \Exception implements \Stringable, \Throwable {
}

final class Psk {
    public const MAX_IDENTITY_LEN = 128;
    public const MAX_PSK_LEN = 256;
    public readonly ?string $identity;
    public readonly string $psk;
    public function __construct(string $psk, ?string $identity = NULL) {}
}

final class Session {
    public readonly string $id;
    public function __serialize(): array {}
    public function __unserialize(array $data): void {}
    public function export(int $format = OPENSSL_ENCODING_PEM): string {}
    public function getCipher(): ?string {}
    public function getCreatedAt(): int {}
    public function getProtocol(): ?string {}
    public function getTicketLifetimeHint(): ?int {}
    public function getTimeout(): int {}
    public function hasTicket(): bool {}
    public static function import(string $data, int $format = OPENSSL_ENCODING_PEM): \Openssl\Session {}
    public function isResumable(): bool {}
}

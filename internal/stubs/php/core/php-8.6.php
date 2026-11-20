<?php

// PHP 8.6 delta: global functions, classes and enums added in 8.6,
// rendered from PHP 8.6.0's reflection. Classes list all their members.

final class IntlNumberRangeFormatter {
    public const COLLAPSE_ALL = 3;
    public const COLLAPSE_AUTO = 0;
    public const COLLAPSE_NONE = 1;
    public const COLLAPSE_UNIT = 2;
    public const IDENTITY_FALLBACK_APPROXIMATELY = 2;
    public const IDENTITY_FALLBACK_APPROXIMATELY_OR_SINGLE_VALUE = 1;
    public const IDENTITY_FALLBACK_RANGE = 3;
    public const IDENTITY_FALLBACK_SINGLE_VALUE = 0;
    private function __construct() {}
    public static function createFromSkeleton(string $skeleton, string $locale, int $collapse, int $identityFallback): \IntlNumberRangeFormatter {}
    public function format(int|float $start, int|float $end): string {}
    public function getErrorCode(): int {}
    public function getErrorMessage(): string {}
}

final class SodiumCryptoXofShake128State {
}

final class SodiumCryptoXofShake256State {
}

final class SodiumCryptoXofTurboshake128State {
}

final class SodiumCryptoXofTurboshake256State {
}

/** The direction of a sort: ascending or descending. */
enum SortDirection {
    case Ascending;
    case Descending;
}

final readonly class StreamError {
    public readonly \StreamErrorCode $code;
    public readonly string $message;
    public readonly int $severity;
    public readonly bool $terminating;
    public readonly string $wrapperName;
}

enum StreamErrorCode {
    case AcceptFailed;
    case AllocationFailed;
    case AlreadyExists;
    case ArchivingFailed;
    case AuthFailed;
    case BindFailed;
    case BufferedDataLost;
    case CastFailed;
    case CastNotSupported;
    case ChmodFailed;
    case ChownFailed;
    case ConnectFailed;
    case CopyFailed;
    case CreateFailed;
    case DecodingFailed;
    case Disabled;
    case DupFailed;
    case EncodingFailed;
    case FilterFailed;
    case FilterNotFound;
    case FlushFailed;
    case Generic;
    case InvalidFormat;
    case InvalidHeader;
    case InvalidMeta;
    case InvalidMode;
    case InvalidParam;
    case InvalidPath;
    case InvalidResponse;
    case InvalidUrl;
    case ListenFailed;
    case LockFailed;
    case LockNotSupported;
    case MakeSeekableFailed;
    case MetaFailed;
    case MkdirFailed;
    case ModeNotSupported;
    case NetworkRecvFailed;
    case NetworkSendFailed;
    case NoOpener;
    case None;
    case NotFound;
    case NotImplemented;
    case NotReadable;
    case NotWritable;
    case OobNotSupported;
    case OpenFailed;
    case PathTooLong;
    case PermissionDenied;
    case PersistentNotSupported;
    case ProtocolError;
    case ProtocolUnsupported;
    case ReadFailed;
    case Readonly;
    case RecursionDetected;
    case RedirectLimit;
    case RenameFailed;
    case ResumptionFailed;
    case RmdirFailed;
    case SeekFailed;
    case SeekNotSupported;
    case SocketPathTooLong;
    case SslNotSupported;
    case StatFailed;
    case TemporaryFileFailed;
    case TimeOut;
    case TouchFailed;
    case TruncateFailed;
    case UnlinkFailed;
    case UserspaceCallFailed;
    case UserspaceInvalidReturn;
    case UserspaceNotImplemented;
    case WrapperDisabled;
    case WrapperNotFound;
    case WrapperRegistrationFailed;
    case WrapperRestorationFailed;
    case WrapperUnregistrationFailed;
    case WriteFailed;
}

enum StreamErrorMode {
    case Error;
    case Exception;
    case Silent;
}

enum StreamErrorStore {
    case All;
    case Auto;
    case NonTerminating;
    case None;
    case Terminating;
}

class StreamException extends \Exception implements \Stringable, \Throwable {
    private array $errors;
    public function getErrors(): array {}
}

final class StreamPollHandle implements \Io\Poll\Handle {
    public function __construct($stream) {}
    public function getStream() {}
    public function isValid(): bool {}
}

/**
 * Returns $value limited to the range from $min to $max: $min when it's
 * below, $max when it's above, and $value itself otherwise.
 *
 * @template T
 * @param T $value
 * @param T $min
 * @param T $max
 * @return T
 */
function clamp(mixed $value, mixed $min, mixed $max): mixed {}

function gmp_powm_sec(\GMP|string|int $num, \GMP|string|int $exponent, \GMP|string|int $modulus): \GMP {}

function gmp_prevprime(\GMP|string|int $num, &$definitely_prime = NULL): \GMP {}

function grapheme_strrev(string $string): string|false {}

function locale_get_display_keyword(string $keyword, ?string $displayLocale = NULL): string|false {}

function locale_get_display_keyword_value(string $locale, string $keyword, ?string $displayLocale = NULL): string|false {}

function mysqli_quote_string(\mysqli $mysql, string $string): string {}

function snmp_init_mib(?string $mibdirs): void {}

function snmp_set_mib_option(\Snmp\Mib $option, bool $enable): void {}

function snmp_set_output_option(\Snmp\Output $option, bool $enable): void {}

function snmp_set_string_output_format(\Snmp\StringOutput $format): void {}

function sodium_bin2ip(string $bin): string {}

function sodium_crypto_ipcrypt_decrypt(string $encrypted_ip, string $key): string {}

function sodium_crypto_ipcrypt_encrypt(string $ip, string $key): string {}

function sodium_crypto_ipcrypt_keygen(): string {}

function sodium_crypto_ipcrypt_nd_decrypt(string $ciphertext_hex, string $key): string {}

function sodium_crypto_ipcrypt_nd_encrypt(string $ip, string $key): string {}

function sodium_crypto_ipcrypt_nd_keygen(): string {}

function sodium_crypto_ipcrypt_ndx_decrypt(string $ciphertext_hex, string $key): string {}

function sodium_crypto_ipcrypt_ndx_encrypt(string $ip, string $key): string {}

function sodium_crypto_ipcrypt_ndx_keygen(): string {}

function sodium_crypto_ipcrypt_pfx_decrypt(string $encrypted_ip, string $key): string {}

function sodium_crypto_ipcrypt_pfx_encrypt(string $ip, string $key): string {}

function sodium_crypto_ipcrypt_pfx_keygen(): string {}

function sodium_crypto_kem_dec(string $ciphertext, string $secret_key): string {}

function sodium_crypto_kem_enc(string $public_key): array {}

function sodium_crypto_kem_keypair(): string {}

function sodium_crypto_kem_mlkem768_dec(string $ciphertext, string $secret_key): string {}

function sodium_crypto_kem_mlkem768_enc(string $public_key): array {}

function sodium_crypto_kem_mlkem768_keypair(): string {}

function sodium_crypto_kem_mlkem768_publickey(string $key_pair): string {}

function sodium_crypto_kem_mlkem768_secretkey(string $key_pair): string {}

function sodium_crypto_kem_mlkem768_seed_keypair(string $seed): string {}

function sodium_crypto_kem_publickey(string $key_pair): string {}

function sodium_crypto_kem_secretkey(string $key_pair): string {}

function sodium_crypto_kem_seed_keypair(string $seed): string {}

function sodium_crypto_xof_shake128(int $length, string $message): string {}

function sodium_crypto_xof_shake128_init(?int $domain = NULL): \SodiumCryptoXofShake128State {}

function sodium_crypto_xof_shake128_squeeze(\SodiumCryptoXofShake128State $state, int $length): string {}

function sodium_crypto_xof_shake128_update(\SodiumCryptoXofShake128State $state, string $message): void {}

function sodium_crypto_xof_shake256(int $length, string $message): string {}

function sodium_crypto_xof_shake256_init(?int $domain = NULL): \SodiumCryptoXofShake256State {}

function sodium_crypto_xof_shake256_squeeze(\SodiumCryptoXofShake256State $state, int $length): string {}

function sodium_crypto_xof_shake256_update(\SodiumCryptoXofShake256State $state, string $message): void {}

function sodium_crypto_xof_turboshake128(int $length, string $message): string {}

function sodium_crypto_xof_turboshake128_init(?int $domain = NULL): \SodiumCryptoXofTurboshake128State {}

function sodium_crypto_xof_turboshake128_squeeze(\SodiumCryptoXofTurboshake128State $state, int $length): string {}

function sodium_crypto_xof_turboshake128_update(\SodiumCryptoXofTurboshake128State $state, string $message): void {}

function sodium_crypto_xof_turboshake256(int $length, string $message): string {}

function sodium_crypto_xof_turboshake256_init(?int $domain = NULL): \SodiumCryptoXofTurboshake256State {}

function sodium_crypto_xof_turboshake256_squeeze(\SodiumCryptoXofTurboshake256State $state, int $length): string {}

function sodium_crypto_xof_turboshake256_update(\SodiumCryptoXofTurboshake256State $state, string $message): void {}

function sodium_ip2bin(string $ip): string {}

function stream_clear_errors(): void {}

/**
 * Returns the errors the last stream operation raised, oldest first.
 *
 * @return list<\StreamError>
 */
function stream_last_errors(): array {}

function stream_socket_get_crypto_status($stream): int {}

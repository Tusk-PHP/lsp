# Optional PECL extension APIs

These are analysis declarations, not executable implementations. They are loaded
only when an extension is selected in the builtin profile (Composer `require`,
`suggest`, `config.platform`, or the existing explicit extension configuration).
The default profile does not select these optional extensions.

`php-7.4.php` denotes the minimum supported PHP profile, not the release date of
the extension API. PECL releases and ImageMagick/libmemcached builds evolve
independently of PHP. The baselines therefore cover their public API families;
we do not invent PHP-version deltas for extension-version differences. A symbol
may still require a newer PECL release or a library build feature at runtime.

The Imagick constants and supporting classes are separate baseline files in
subdirectories to keep each file below 800 lines. The existing builtin layer
merging combines the repeated `Imagick` class declarations.

## Primary sources

Signatures were adapted from the extension maintainers' own Zend declarations,
not from PhpStorm or another third-party stub collection:

- [Imagick](https://github.com/Imagick/imagick): `Imagick.stub.php`,
  `ImagickDraw.stub.php`, `ImagickPixel.stub.php`,
  `ImagickPixelIterator.stub.php`, `ImagickKernel.stub.php`, and constant
  registrations in `imagick_helpers.c`. Includes ImageMagick 6 and 7 APIs.
- [phpredis 6.0.2](https://github.com/phpredis/phpredis/tree/6.0.2):
  `redis.stub.php`, `redis_array.stub.php`, `redis_cluster.stub.php`,
  `redis_sentinel.stub.php`, and constant values in `common.h`/`library.h`.
  Includes the retained 5.x aliases `delete`, `open`, and `popen`.
- [Memcached](https://github.com/php-memcached-dev/php-memcached):
  `php_memcached.stub.php` and constant registrations in `php_memcached.c`.
  The separate optional `MemcachedServer` protocol extension is outside this set.
- [APCu](https://github.com/krakjoe/apcu): `php_apc.stub.php`,
  `apc_iterator.stub.php`, `apc_iterator.h`. Debug-only functions are excluded.
- [SSH2](https://github.com/php/pecl-networking-ssh2): argument metadata,
  parameter parsing, and constant registrations in `ssh2.c`, `ssh2_sftp.c`,
  `ssh2_fopen_wrappers.c`, and `php_ssh2.h`; libssh2 values from its
  [public header](https://github.com/libssh2/libssh2/blob/master/include/libssh2.h).
- [igbinary](https://github.com/igbinary/igbinary/blob/master/igbinary.php):
  the extension's own function documentation.

`resource` is a PHPDoc type rather than a PHP native type; SSH2 uses `@param`
and `@return` for these signatures. Imagick quantum values use `int|float` to
cover fixed-point and HDRI builds. Redis values can be `mixed` when serializers
are enabled.

`TODO(verify)` flags unresolved build-dependent constant values (represented
by `UNKNOWN`, with their actual declared type), and the optional `z` coordinate
used to combine ImageMagick 6/7 primary-color signatures. No runtime constant
value should be inferred from an `UNKNOWN` placeholder.

## Coverage and loading cost

The symbol tests assert these declaration counts, including members merged
from the split Imagick baselines:

| Extension | Classes | Methods | Functions | Constants |
| --- | ---: | ---: | ---: | ---: |
| imagick | 10 | 607 | 0 | 661 |
| redis | 6 | 519 | 0 | 50 |
| memcached | 2 | 59 | 0 | 150 |
| apcu | 1 | 9 | 14 | 15 |
| ssh2 | 0 | 0 | 41 | 29 |
| igbinary | 0 | 0 | 2 | 0 |

The selection and availability tests check every optional top-level declaration
is absent from a core-only index. Unselected source files are never read or
parsed; only the embedded binary data and small availability table are shared.

`BenchmarkRegisterOptionalExtensionBuiltins` (`-benchmem -count=3`, Apple M3 Max,
PHP 7.4 profile) measured core-only registration at 5.02–5.06 ms and 2.547 MB
allocated before this change, and 4.70–4.89 ms and 2.547 MB afterward. All six
extensions selected measured 15.03–16.25 ms and 10.777 MB allocated. These are
registration allocations, not retained heap measurements. Core-only allocation
increased by about 400 bytes for enumeration of the additional embedded paths;
no optional extension symbols or source are retained in that profile.

## Notices

API declarations adapted from Imagick, phpredis, Memcached and APCu are covered
by the PHP License 3.01, reproduced in [LICENSE-PHP](LICENSE-PHP). Copyright
notices from their source distributions:

- Imagick: Copyright (c) 2006–2013 Mikko Koppanen, Scott MacVicar.
  ImageMagick (c) ImageMagick Studio LLC.
- phpredis: Copyright (c) 1997–2009 The PHP Group.
- Memcached: Copyright (c) 2009–2010 The PHP Group.
- APCu: Copyright (c) 2006–2011 The PHP Group. See [NOTICE-APCu](NOTICE-APCu)
  for the original-contributor notice.

The remaining signatures and descriptions were written from the documented
API behavior. No third-party stub-set code or prose is included.

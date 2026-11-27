<?php

declare(strict_types=1);

function constantDumpCode(): string
{
    return <<<'PHP'
$groups = get_defined_constants(true);
unset($groups['user']);
$out = ['version' => PHP_MAJOR_VERSION . '.' . PHP_MINOR_VERSION, 'extensions' => array_map('strtolower', get_loaded_extensions()), 'constants' => []];
foreach ($groups as $extension => $constants) {
    foreach ($constants as $name => $value) {
        $out['constants'][strtolower($extension)][$name] = [gettype($value), is_scalar($value) || $value === null ? var_export($value, true) : null];
    }
}
echo json_encode($out, JSON_INVALID_UTF8_SUBSTITUTE);
PHP;
}

/**
 * Constants whose value depends on the platform, not on the PHP version. Like
 * the versions of PHP and of the libraries it was built with, which any name
 * containing VERSION is taken to be, their type is kept and their value is not.
 */
function platformConstants(): array
{
    return [
        'DIRECTORY_SEPARATOR', 'PATH_SEPARATOR', 'PHP_EOL', 'PHP_OS', 'PHP_OS_FAMILY',
        'PHP_SHLIB_SUFFIX', 'PHP_MAXPATHLEN', 'PHP_FD_SETSIZE', 'PHP_BINARY', 'PHP_SAPI',
    ];
}

/**
 * Extensions bundled with PHP that wrap no outside library, so the constants
 * they define depend on the PHP version alone. In every official image from
 * 7.4 to 8.6 each of their constants, once added, stays, and the additions
 * match PHP's changelogs (T_MATCH 8.0, MHASH_XXH3 8.1, DATE_ISO8601_EXPANDED
 * 8.2, PHP_SBINDIR 8.4, ...). Extensions around a library (curl, openssl,
 * sodium, libxml, posix, sockets, ...) gain constants with the library's
 * version or the platform, which the images also changed.
 */
function versionOnlyExtensions(): array
{
    return ['core', 'standard', 'date', 'json', 'pcre', 'spl', 'tokenizer', 'hash', 'filter', 'random', 'reflection', 'session'];
}

/**
 * Constants of those extensions defined only when the C library or the build
 * provides them: the nl_langinfo() and localeconv() items, which first appear
 * in the 8.2 images (built on a newer C library) although PHP knew them long
 * before, and PHP_BUILD_PROVIDER, set only when the build names a provider.
 */
function platformConditionalConstants(): array
{
    return [
        'CURRENCY_SYMBOL', 'DECIMAL_POINT', 'ERA_YEAR', 'FRAC_DIGITS', 'GROUPING', 'INT_CURR_SYMBOL',
        'INT_FRAC_DIGITS', 'MON_DECIMAL_POINT', 'MON_GROUPING', 'MON_THOUSANDS_SEP', 'NEGATIVE_SIGN',
        'NOSTR', 'N_CS_PRECEDES', 'N_SEP_BY_SPACE', 'N_SIGN_POSN', 'POSITIVE_SIGN', 'P_CS_PRECEDES',
        'P_SEP_BY_SPACE', 'P_SIGN_POSN', 'THOUSANDS_SEP', 'YESSTR', 'PHP_BUILD_PROVIDER',
    ];
}

/**
 * Whether builtin-unavailable may report the constant on an older PHP: its
 * presence depends on the PHP version alone.
 */
function isVersionOnly(string $name, string $extension): bool
{
    return in_array($extension, versionOnlyExtensions(), true)
        && !in_array($name, platformConditionalConstants(), true)
        && !in_array($name, platformConstants(), true);
}

function isBuildDependent(string $name): bool
{
    return in_array($name, platformConstants(), true) || str_contains($name, 'VERSION');
}

/**
 * Constants PHP's grammar handles itself, which are never looked up by name.
 */
function languageConstants(): array
{
    return ['TRUE', 'FALSE', 'NULL'];
}

/**
 * @return list<array{version:string, kind:string, extensions:list<string>, constants:array<string, array<string, array{0:string, 1:?string}>>}>
 */
function dumpConstantsWith(string $commands, string $kind): array
{
    return array_map(static fn(array $dump): array => $dump + ['kind' => $kind], runPhpDumps($commands, constantDumpCode()));
}

/**
 * @param list<array{version:string, kind:string, extensions:list<string>, constants:array<string, array<string, array{0:string, 1:?string}>>}> $dumps
 */
function generateGoConstants(array $dumps): string
{
    if ($dumps === []) {
        fwrite(STDERR, "Constants mode needs at least one PHP (--php).\n");
        exit(1);
    }
    usort($dumps, static fn(array $a, array $b): int => version_compare($a['version'], $b['version']));
    $baseline = $dumps[0]['version'];
    $peclExtensions = peclExtensions($dumps);

    $seen = [];
    foreach ($dumps as $dump) {
        foreach ($dump['constants'] as $extension => $constants) {
            foreach ($constants as $name => [$type, $value]) {
                if (in_array($name, languageConstants(), true) || str_starts_with($name, '__')) {
                    continue;
                }
                $seen[$name][] = ['dump' => $dump, 'extension' => $extension, 'type' => phpTypeName($type), 'value' => $value];
            }
        }
    }
    ksort($seen, SORT_STRING);

    $lines = [];
    foreach ($seen as $name => $occurrences) {
        $latest = $occurrences[count($occurrences) - 1];
        $extension = $latest['extension'];
        $types = array_unique(array_column($occurrences, 'type'));
        $values = array_unique(array_map(static fn(array $o): string => (string) $o['value'], $occurrences));
        $type = count($types) === 1 ? $types[0] : '';
        $value = count($values) === 1 && $occurrences[0]['value'] !== null && !isBuildDependent($name) ? $occurrences[0]['value'] : '';
        $since = sinceVersion($name, $occurrences, $dumps, $baseline);
        $gate = isset($peclExtensions[$extension]) ? $extension : '';
        $lines[] = sprintf(
            "\t{Name: %s, Type: %s, Value: %s, Warnable: %s, BuiltinAvailability: BuiltinAvailability{Since: %s, Extension: %s}},",
            renderGoString($name),
            renderGoString($type),
            renderGoValue($value),
            isVersionOnly($name, $extension) ? 'true' : 'false',
            renderGoString($since),
            renderGoString($gate)
        );
    }

    $versions = implode(', ', array_map(static fn(array $d): string => $d['version'] . ($d['kind'] === 'default' ? '' : " with {$d['kind']} extensions"), $dumps));

    return implode("\n", [
        'package symbols',
        '',
        '// Code generated from PHP runtime builtin symbols; DO NOT EDIT.',
        "// Generated by scripts/generate-builtins.php constants from get_defined_constants(true) of PHP {$versions}.",
        '',
        'var generatedBuiltinConstants = []builtinConstant{',
        ...$lines,
        '}',
        '',
    ]);
}

/**
 * Extensions only the PECL builds have. Their constants need the extension in
 * the project's profile; those of PHP's own extensions are not gated, like
 * their functions and classes.
 *
 * @return array<string, true>
 */
function peclExtensions(array $dumps): array
{
    $own = [];
    $pecl = [];
    foreach ($dumps as $dump) {
        foreach ($dump['extensions'] as $extension) {
            if ($dump['kind'] === 'pecl') {
                $pecl[$extension] = true;
            } else {
                $own[$extension] = true;
            }
        }
    }
    return array_diff_key($pecl, $own);
}

/**
 * The version that added name: the first one that has it, when an earlier
 * build had its extension without it. A constant of an extension no earlier
 * build had is taken to be as old as the baseline.
 */
function sinceVersion(string $name, array $occurrences, array $dumps, string $baseline): string
{
    $first = $occurrences[0]['dump']['version'];
    if ($first === $baseline) {
        return '';
    }
    $extensions = array_unique(array_column($occurrences, 'extension'));
    foreach ($dumps as $dump) {
        if (version_compare($dump['version'], $first, '>=')) {
            break;
        }
        if (array_intersect($extensions, $dump['extensions']) !== []) {
            return $first;
        }
    }
    return '';
}

function renderGoValue(string $value): string
{
    return json_encode($value, JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_INVALID_UTF8_SUBSTITUTE | JSON_THROW_ON_ERROR);
}

return [
    'name' => 'constants',
    'usage' => 'php scripts/generate-builtins.php constants --php=COMMANDS [--extensions-php=COMMANDS] [--pecl-php=COMMANDS] [--output=PATH]',
    'help' => '  constants     Run each PHP given with --php and --extensions-php, read its
                get_defined_constants(true), and emit the Go table of PHP\'s
                global constants with their type, value, the version that
                added them and, for PECL extensions, the extension. Output
                defaults to
                internal/symbols/builtin_constants_generated.go.
',
    'run' => static function (array $options): void {
        writeOutput(
            generateGoConstants(array_merge(
                dumpConstantsWith($options['php'], 'default'),
                dumpConstantsWith($options['extensions-php'], 'extensions'),
                dumpConstantsWith($options['pecl-php'], 'pecl')
            )),
            $options['output'] ?? dirname(__DIR__, 2) . '/internal/symbols/builtin_constants_generated.go'
        );
    },
];

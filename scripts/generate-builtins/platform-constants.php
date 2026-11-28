<?php

declare(strict_types=1);

/**
 * The directory of the per-platform constant values the constants mode reads,
 * one file per platform other than Linux.
 */
function platformConstantsDir(): string
{
    return __DIR__ . '/platform-constants';
}

/**
 * The dumps in $dir, written by dumps/constants.php under builds of one
 * platform, oldest version first.
 *
 * @return list<array{version:string, release:string, os:string, extensions:list<string>, constants:array<string, array<string, array{0:string, 1:?string}>>}>
 */
function readConstantDumps(string $dir): array
{
    $files = glob($dir . '/*.json');
    if ($files === false || $files === []) {
        fwrite(STDERR, "No constant dumps (*.json) in {$dir}\n");
        exit(1);
    }
    $dumps = [];
    foreach ($files as $file) {
        try {
            $dumps[] = json_decode((string) file_get_contents($file), true, 512, JSON_THROW_ON_ERROR);
        } catch (JsonException $e) {
            fwrite(STDERR, "Unreadable dump {$file}: {$e->getMessage()}\n");
            exit(1);
        }
    }
    usort($dumps, static fn(array $a, array $b): int => version_compare($a['release'], $b['release']));
    return $dumps;
}

/**
 * One platform's constant values: each constant's value when all its builds
 * agree on it, null when they don't or it isn't scalar.
 *
 * @param list<array{version:string, release:string, os:string, extensions:list<string>, constants:array<string, array<string, array{0:string, 1:?string}>>}> $dumps
 * @return array{os:string, builds:list<string>, values:array<string, ?string>}
 */
function platformConstantValues(array $dumps): array
{
    $systems = array_values(array_unique(array_column($dumps, 'os')));
    if (count($systems) !== 1) {
        fwrite(STDERR, 'The dumps of one platform come from one OS, not ' . implode(', ', $systems) . "\n");
        exit(1);
    }
    $values = [];
    foreach ($dumps as $dump) {
        foreach ($dump['constants'] as $constants) {
            foreach ($constants as $name => [, $value]) {
                $values[$name] = array_key_exists($name, $values) && $values[$name] !== $value ? null : $value;
            }
        }
    }
    ksort($values, SORT_STRING);
    $builds = array_map(
        static fn(array $d): string => $d['release'] . ' (' . count($d['extensions']) . ' extensions: ' . implode(' ', $d['extensions']) . ')',
        $dumps
    );
    return ['os' => $systems[0], 'builds' => $builds, 'values' => $values];
}

/**
 * The values of every platform in platformConstantsDir().
 *
 * @return list<array{os:string, builds:list<string>, values:array<string, ?string>}>
 */
function readPlatformConstantValues(): array
{
    $files = glob(platformConstantsDir() . '/*.json');
    if ($files === false) {
        fwrite(STDERR, "Can't list " . platformConstantsDir() . "\n");
        exit(1);
    }
    sort($files, SORT_STRING);
    return array_map(static function (string $file): array {
        try {
            return json_decode((string) file_get_contents($file), true, 512, JSON_THROW_ON_ERROR);
        } catch (JsonException $e) {
            fwrite(STDERR, "Unreadable platform values {$file}: {$e->getMessage()}\n");
            exit(1);
        }
    }, $files);
}

return [
    'name' => 'platform-constants',
    'usage' => 'php scripts/generate-builtins.php platform-constants --dumps=DIR --output=PATH',
    'help' => '  platform-constants
                Read the dumps in --dumps that one platform\'s builds wrote
                with scripts/generate-builtins/dumps/constants.php, and write
                the values they agree on, the file the constants mode reads
                from scripts/generate-builtins/platform-constants/ to drop a
                value that differs between platforms.
',
    'run' => static function (array $options): void {
        if ($options['dumps'] === null || $options['output'] === null) {
            fwrite(STDERR, "platform-constants needs --dumps and --output.\n");
            exit(1);
        }
        $json = json_encode(
            platformConstantValues(readConstantDumps($options['dumps'])),
            JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_INVALID_UTF8_SUBSTITUTE | JSON_THROW_ON_ERROR
        );
        writeOutput($json . "\n", $options['output']);
    },
];

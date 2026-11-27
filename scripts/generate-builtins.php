#!/usr/bin/env php
<?php

declare(strict_types=1);

if (PHP_SAPI !== 'cli') {
    fwrite(STDERR, "This script must be run from the command line.\n");
    exit(1);
}

main($argv);

function main(array $argv): void
{
    array_shift($argv);

    if ($argv === [] || in_array($argv[0], ['-h', '--help'], true)) {
        fwrite(STDOUT, usage());
        return;
    }

    $mode = array_shift($argv);
    $modes = modes();
    if (!in_array($mode, ['registry', 'stubs'], true) && !isset($modes[$mode])) {
        fwrite(STDERR, "Unknown mode: {$mode}\n\n");
        fwrite(STDERR, usage());
        exit(1);
    }

    $options = parseOptions($argv);

    if (isset($modes[$mode])) {
        ($modes[$mode]['run'])($options);
        return;
    }

    $symbols = collectBuiltinSymbols();

    $output = match ($mode) {
        'registry' => generateGoRegistry(...keepRegistryNames($symbols, $options['keep'])),
        'stubs' => generatePhpStubs($symbols['functions'], $symbols['classLikes']),
    };

    writeOutput($output, $options['output']);
}

/**
 * The modes in scripts/generate-builtins/, one file each. A file defines the
 * mode's functions and returns its name, its usage line, its help and the
 * function that runs it with the parsed options.
 *
 * @return array<string, array{name:string, usage:string, help:string, run:callable(array):void}>
 */
function modes(): array
{
    static $modes = null;
    if ($modes !== null) {
        return $modes;
    }
    $modes = [];
    $files = glob(__DIR__ . '/generate-builtins/*.php');
    if ($files === false) {
        fwrite(STDERR, "Can't list " . __DIR__ . "/generate-builtins\n");
        exit(1);
    }
    sort($files, SORT_STRING);
    foreach ($files as $file) {
        $mode = require $file;
        $modes[$mode['name']] = $mode;
    }
    return $modes;
}

function usage(): string
{
    $modeUsage = implode('', array_map(static fn(array $m): string => '  ' . $m['usage'] . "\n", modes()));
    $modeHelp = implode('', array_map(static fn(array $m): string => $m['help'], modes()));

    return <<<TXT
Usage:
  php scripts/generate-builtins.php registry [--keep=PATH] [--output=PATH]
  php scripts/generate-builtins.php stubs [--output=PATH]
{$modeUsage}
Modes:
  registry      Emit the Go builtin name registry from the local PHP runtime.
  stubs         Emit simple global PHP builtin stub declarations from runtime reflection.
{$modeHelp}
Options:
  --keep=PATH    Registry mode: also keep every name already listed in this
                 generated Go registry, so names from extensions the running
                 PHP doesn't have (redis, brotli, ...) aren't dropped.
  --output=PATH  Write to PATH instead of the mode's default (stdout for
                 registry and stubs).
  --php=COMMANDS PHP's default builds, one per supported version, for the modes
                 that read several builds: commands separated by ';', each split
                 on spaces and run as an argv array, e.g.
                 'docker run --rm php:7.4-cli php;docker run --rm php:8.6-cli php'.
  --extensions-php=COMMANDS
                 More builds, with the extensions of PHP's own source the
                 default builds lack (intl, gd, ffi, ...), in the same form.
  --pecl-php=COMMANDS
                 Builds with PECL extensions (redis, brotli, ...). What only
                 these builds have needs its extension in the project's profile.
  --phpstan=PATH A phpstan-src checkout, whose resources/functionMap*.php
                 signatures mode reads.
  -h, --help     Show this help.

TXT;
}

/**
 * @return array{output:?string, keep:?string, php:string, extensions-php:string, pecl-php:string, phpstan:?string}
 */
function parseOptions(array $argv): array
{
    $options = ['output' => null, 'keep' => null, 'php' => '', 'extensions-php' => '', 'pecl-php' => '', 'phpstan' => null];

    foreach ($argv as $arg) {
        if (str_starts_with($arg, '--output=')) {
            $options['output'] = substr($arg, strlen('--output='));
            continue;
        }

        if (str_starts_with($arg, '--phpstan=')) {
            $options['phpstan'] = substr($arg, strlen('--phpstan='));
            continue;
        }

        if (str_starts_with($arg, '--keep=')) {
            $options['keep'] = substr($arg, strlen('--keep='));
            continue;
        }

        foreach (['php', 'extensions-php', 'pecl-php'] as $builds) {
            if (str_starts_with($arg, "--{$builds}=")) {
                $options[$builds] = substr($arg, strlen("--{$builds}="));
                continue 2;
            }
        }

        fwrite(STDERR, "Unknown option: {$arg}\n\n");
        fwrite(STDERR, usage());
        exit(1);
    }

    return $options;
}

/**
 * @return array{functions:list<string>, classLikes:list<string>}
 */
function collectBuiltinSymbols(): array
{
    $defined = get_defined_functions();
    $functions = $defined['internal'] ?? [];
    $functions = array_values(array_unique(array_filter($functions, 'is_string')));
    sort($functions, SORT_STRING);

    $classLikes = array_merge(
        array_filter(get_declared_classes(), 'isBuiltinClassLike'),
        array_filter(get_declared_interfaces(), 'isBuiltinClassLike'),
        array_filter(get_declared_traits(), 'isBuiltinClassLike')
    );
    $classLikes = array_values(array_unique($classLikes));
    sort($classLikes, SORT_STRING);

    return [
        'functions' => $functions,
        'classLikes' => $classLikes,
    ];
}

function isBuiltinClassLike(string $name): bool
{
    try {
        return (new ReflectionClass($name))->isInternal();
    } catch (ReflectionException) {
        return false;
    }
}

/**
 * Adds the names an existing generated registry lists to the running PHP's,
 * so regenerating on a build without some extensions doesn't drop them.
 *
 * @param array{functions:list<string>, classLikes:list<string>} $symbols
 * @return array{functions:list<string>, classLikes:list<string>, kept:?string}
 */
function keepRegistryNames(array $symbols, ?string $keepPath): array
{
    if ($keepPath === null) {
        return ['functions' => $symbols['functions'], 'classLikes' => $symbols['classLikes'], 'kept' => null];
    }
    $source = @file_get_contents($keepPath);
    if ($source === false) {
        fwrite(STDERR, "Cannot read {$keepPath}\n");
        exit(1);
    }
    $lists = [];
    foreach (['functions' => 'generatedBuiltinFunctionNames', 'classLikes' => 'generatedBuiltinClassLikeNames'] as $key => $var) {
        if (!preg_match('/' . $var . ' = \[\]string\{(.*?)\n\}/s', $source, $block)) {
            fwrite(STDERR, "{$keepPath} has no {$var} list\n");
            exit(1);
        }
        preg_match_all('/"((?:[^"\\\\]|\\\\.)*)"/', $block[1], $names);
        $kept = array_map('stripcslashes', $names[1]);
        $merged = array_values(array_unique(array_merge($symbols[$key], $kept)));
        sort($merged, SORT_STRING);
        $lists[$key] = $merged;
    }

    return ['functions' => $lists['functions'], 'classLikes' => $lists['classLikes'], 'kept' => basename($keepPath)];
}

/**
 * @param list<string> $functions
 * @param list<string> $classLikes
 */
function generateGoRegistry(array $functions, array $classLikes, ?string $kept = null): string
{
    $phpVersion = PHP_VERSION;

    $parts = [
        'package symbols',
        '',
        '// Code generated from PHP runtime builtin symbols; DO NOT EDIT.',
        sprintf(
            '// Generated with PHP %s using get_defined_functions(), get_declared_classes(), get_declared_interfaces(), and get_declared_traits().',
            $phpVersion
        ),
        ...($kept === null ? [] : ['// Names from extensions this PHP build lacks are kept from the previous registry (--keep).']),
        '',
        'var generatedBuiltinFunctionNames = []string{',
        renderGoStringList($functions),
        '}',
        '',
        'var generatedBuiltinClassLikeNames = []string{',
        renderGoStringList($classLikes),
        '}',
        '',
    ];

    return implode("\n", $parts);
}

/**
 * @param list<string> $values
 */
function renderGoStringList(array $values): string
{
    $lines = [];
    foreach ($values as $value) {
        $lines[] = "\t" . renderGoString($value) . ',';
    }

    return implode("\n", $lines);
}

function renderGoString(string $value): string
{
    return '"' . addcslashes($value, "\\\"\n\r\t") . '"';
}

/**
 * @param list<string> $functions
 * @param list<string> $classLikes
 */
function generatePhpStubs(array $functions, array $classLikes): string
{
    $namespaces = [];
    $skippedFunctions = [];
    $skippedClassLikes = [];

    foreach ($functions as $name) {
        if (!isDeclarableFunctionName($name)) {
            $skippedFunctions[] = $name;
            continue;
        }

        try {
            $reflection = new ReflectionFunction($name);
        } catch (ReflectionException) {
            $skippedFunctions[] = $name;
            continue;
        }

        if (!$reflection->isInternal()) {
            continue;
        }

        [$namespace, $shortName] = splitName($name);
        if ($namespace !== '') {
            $skippedFunctions[] = $name;
            continue;
        }

        $namespaces[$namespace]['functions'][] = renderFunctionStub($reflection, $shortName);
    }

    foreach ($classLikes as $name) {
        try {
            $reflection = new ReflectionClass($name);
        } catch (ReflectionException) {
            continue;
        }

        if (!$reflection->isInternal()) {
            continue;
        }

        [$namespace, $shortName] = splitName($name);
        if ($namespace !== '') {
            $skippedClassLikes[] = $name;
            continue;
        }

        $namespaces[$namespace]['classLikes'][] = renderClassLikeStub($reflection, $shortName);
    }

    ksort($namespaces, SORT_STRING);
    foreach ($namespaces as &$namespace) {
        $namespace['functions'] = $namespace['functions'] ?? [];
        $namespace['classLikes'] = $namespace['classLikes'] ?? [];
        sort($namespace['functions'], SORT_STRING);
        sort($namespace['classLikes'], SORT_STRING);
    }
    unset($namespace);

    $lines = [
        '<?php',
        '',
        '// Generated from the local PHP runtime; DO NOT EDIT.',
        sprintf('// Generated with PHP %s via runtime reflection.', PHP_VERSION),
    ];

    if ($skippedFunctions !== []) {
        sort($skippedFunctions, SORT_STRING);
        $lines[] = '// Skipped non-global, non-declarable, or non-reflectable internal functions: ' . implode(', ', $skippedFunctions) . '.';
    }

    if ($skippedClassLikes !== []) {
        sort($skippedClassLikes, SORT_STRING);
        $lines[] = '// Skipped non-global or non-reflectable internal class-like symbols: ' . implode(', ', $skippedClassLikes) . '.';
    }

    $lines[] = '';

    foreach ($namespaces as $namespace => $members) {
        foreach ($members['functions'] as $stub) {
            $lines[] = $stub;
            $lines[] = '';
        }

        foreach ($members['classLikes'] as $stub) {
            $lines[] = $stub;
            $lines[] = '';
        }
    }

    if (end($lines) === '') {
        array_pop($lines);
    }

    $lines[] = '';

    return implode("\n", $lines);
}

function isDeclarableFunctionName(string $name): bool
{
    $segments = explode('\\', $name);
    foreach ($segments as $segment) {
        if ($segment === '' || !preg_match('/^[A-Za-z_\x80-\xff][A-Za-z0-9_\x80-\xff]*$/', $segment)) {
            return false;
        }

        if (isReservedKeyword(strtolower($segment))) {
            return false;
        }
    }

    return true;
}

function isReservedKeyword(string $word): bool
{
    static $keywords = [
        '__halt_compiler', 'abstract', 'and', 'array', 'as', 'break', 'callable', 'case',
        'catch', 'class', 'clone', 'const', 'continue', 'declare', 'default', 'die', 'do',
        'echo', 'else', 'elseif', 'empty', 'enddeclare', 'endfor', 'endforeach', 'endif',
        'endswitch', 'endwhile', 'enum', 'eval', 'exit', 'extends', 'final', 'finally',
        'fn', 'for', 'foreach', 'from', 'function', 'global', 'goto', 'if', 'implements',
        'include', 'include_once', 'instanceof', 'insteadof', 'interface', 'isset', 'list',
        'match', 'namespace', 'new', 'or', 'print', 'private', 'protected', 'public',
        'readonly', 'require', 'require_once', 'return', 'self', 'static', 'switch', 'throw',
        'trait', 'try', 'unset', 'use', 'var', 'while', 'xor', 'yield',
    ];

    return in_array($word, $keywords, true);
}

/**
 * @return array{0:string, 1:string}
 */
function splitName(string $name): array
{
    $position = strrpos($name, '\\');
    if ($position === false) {
        return ['', $name];
    }

    return [substr($name, 0, $position), substr($name, $position + 1)];
}

function renderFunctionStub(ReflectionFunction $function, string $shortName): string
{
    $signature = 'function ';

    if ($function->returnsReference()) {
        $signature .= '&';
    }

    $signature .= $shortName;
    $signature .= '(' . implode(', ', array_map(renderParameterStub(...), $function->getParameters())) . ')';

    $returnType = getEffectiveReturnType($function);
    if ($returnType !== null) {
        $signature .= ': ' . formatPhpType($returnType);
    }

    return $signature . ' {}';
}

function renderClassLikeStub(ReflectionClass $class, string $shortName): string
{
    if (method_exists($class, 'isEnum') && $class->isEnum()) {
        return renderEnumStub($class, $shortName);
    }

    $headerParts = [];
    if (!$class->isInterface() && !$class->isTrait()) {
        if ($class->isAbstract()) {
            $headerParts[] = 'abstract';
        }
        if ($class->isFinal()) {
            $headerParts[] = 'final';
        }
        if (method_exists($class, 'isReadOnly') && $class->isReadOnly()) {
            $headerParts[] = 'readonly';
        }
    }

    if ($class->isInterface()) {
        $headerParts[] = 'interface';
    } elseif ($class->isTrait()) {
        $headerParts[] = 'trait';
    } else {
        $headerParts[] = 'class';
    }

    $headerParts[] = $shortName;

    if (!$class->isInterface() && !$class->isTrait()) {
        $parent = $class->getParentClass();
        if ($parent !== false) {
            $headerParts[] = 'extends ' . qualifyTypeName($parent->getName());
        }
    }

    $implements = renderInterfaceList($class);
    if ($implements !== '') {
        $headerParts[] = $class->isInterface() ? 'extends ' . $implements : 'implements ' . $implements;
    }

    $lines = [implode(' ', $headerParts) . ' {'];

    foreach (renderClassConstants($class) as $constant) {
        $lines[] = '    ' . $constant;
    }

    foreach (renderProperties($class) as $property) {
        $lines[] = '    ' . $property;
    }

    foreach (renderMethods($class) as $method) {
        foreach (explode("\n", $method) as $line) {
            $lines[] = '    ' . $line;
        }
    }

    $lines[] = '}';

    return implode("\n", $lines);
}

function renderEnumStub(ReflectionClass $class, string $shortName): string
{
    $header = ['enum', $shortName];

    if (is_subclass_of($class->getName(), BackedEnum::class)) {
        $firstCase = $class->getReflectionConstants()[0] ?? null;
        if ($firstCase instanceof ReflectionClassConstant && method_exists($firstCase, 'getValue')) {
            $caseValue = $firstCase->getValue();
            if (is_string($caseValue)) {
                $header[] = ': string';
            } elseif (is_int($caseValue)) {
                $header[] = ': int';
            }
        }
    }

    $implements = renderInterfaceList($class);
    if ($implements !== '') {
        $header[] = 'implements ' . $implements;
    }

    $lines = [implode(' ', $header) . ' {'];

    if (method_exists($class, 'getCases')) {
        $cases = $class->getCases();
        usort($cases, static fn(ReflectionClassConstant $a, ReflectionClassConstant $b): int => strcmp($a->getName(), $b->getName()));

        foreach ($cases as $case) {
            $line = 'case ' . $case->getName();
            if (method_exists($case, 'getBackingValue')) {
                $line .= ' = ' . renderPhpValue($case->getBackingValue());
            }
            $lines[] = '    ' . $line . ';';
        }
    }

    foreach (renderClassConstants($class, true) as $constant) {
        $lines[] = '    ' . $constant;
    }

    foreach (renderProperties($class) as $property) {
        $lines[] = '    ' . $property;
    }

    foreach (renderMethods($class) as $method) {
        foreach (explode("\n", $method) as $line) {
            $lines[] = '    ' . $line;
        }
    }

    $lines[] = '}';

    return implode("\n", $lines);
}

function renderInterfaceList(ReflectionClass $class): string
{
    $names = $class->getInterfaceNames();
    sort($names, SORT_STRING);

    $names = array_values(array_filter($names, static fn(string $name): bool => $name !== $class->getName()));
    if ($names === []) {
        return '';
    }

    return implode(', ', array_map(qualifyTypeName(...), $names));
}

/**
 * @return list<string>
 */
function renderClassConstants(ReflectionClass $class, bool $skipEnumCases = false): array
{
    $constants = array_filter(
        $class->getReflectionConstants(),
        static fn(ReflectionClassConstant $constant): bool => $constant->getDeclaringClass()->getName() === $class->getName()
    );

    usort(
        $constants,
        static fn(ReflectionClassConstant $a, ReflectionClassConstant $b): int => strcmp($a->getName(), $b->getName())
    );

    $lines = [];
    foreach ($constants as $constant) {
        if ($skipEnumCases && method_exists($constant, 'isEnumCase') && $constant->isEnumCase()) {
            continue;
        }

        $parts = [];
        if (!$class->isInterface()) {
            if ($constant->isPublic()) {
                $parts[] = 'public';
            } elseif ($constant->isProtected()) {
                $parts[] = 'protected';
            } else {
                $parts[] = 'private';
            }

            if ($constant->isFinal()) {
                $parts[] = 'final';
            }
        }

        $parts[] = 'const';
        $parts[] = $constant->getName() . ' = ' . renderPhpValue($constant->getValue()) . ';';
        $lines[] = implode(' ', $parts);
    }

    return $lines;
}

/**
 * @return list<string>
 */
function renderProperties(ReflectionClass $class): array
{
    $properties = array_filter(
        $class->getProperties(),
        static fn(ReflectionProperty $property): bool => $property->getDeclaringClass()->getName() === $class->getName()
    );

    usort(
        $properties,
        static fn(ReflectionProperty $a, ReflectionProperty $b): int => strcmp($a->getName(), $b->getName())
    );

    $lines = [];
    foreach ($properties as $property) {
        $parts = [];

        if ($property->isPublic()) {
            $parts[] = 'public';
        } elseif ($property->isProtected()) {
            $parts[] = 'protected';
        } else {
            $parts[] = 'private';
        }

        if ($property->isStatic()) {
            $parts[] = 'static';
        }

        if (method_exists($property, 'isReadOnly') && $property->isReadOnly()) {
            $parts[] = 'readonly';
        }

        $type = $property->getType();
        if ($type !== null) {
            $parts[] = formatPhpType($type);
        }

        $parts[] = '$' . $property->getName() . ';';
        $lines[] = implode(' ', $parts);
    }

    return $lines;
}

/**
 * @return list<string>
 */
function renderMethods(ReflectionClass $class): array
{
    $methods = array_filter(
        $class->getMethods(),
        static fn(ReflectionMethod $method): bool => $method->getDeclaringClass()->getName() === $class->getName()
    );

    usort(
        $methods,
        static fn(ReflectionMethod $a, ReflectionMethod $b): int => strcmp($a->getName(), $b->getName())
    );

    $lines = [];
    foreach ($methods as $method) {
        $parts = [];

        if ($method->isPublic()) {
            $parts[] = 'public';
        } elseif ($method->isProtected()) {
            $parts[] = 'protected';
        } else {
            $parts[] = 'private';
        }

        if ($method->isAbstract()) {
            $parts[] = 'abstract';
        }

        if ($method->isFinal()) {
            $parts[] = 'final';
        }

        if ($method->isStatic()) {
            $parts[] = 'static';
        }

        $parts[] = 'function';

        if ($method->returnsReference()) {
            $parts[] = '&' . $method->getName();
        } else {
            $parts[] = $method->getName();
        }

        $signature = implode(' ', $parts);
        $signature .= '(' . implode(', ', array_map(renderParameterStub(...), $method->getParameters())) . ')';

        $returnType = getEffectiveReturnType($method);
        if ($returnType !== null) {
            $signature .= ': ' . formatPhpType($returnType);
        }

        if ($method->isAbstract() || $class->isInterface()) {
            $lines[] = $signature . ';';
            continue;
        }

        $lines[] = $signature . ' {}';
    }

    return $lines;
}

function renderParameterStub(ReflectionParameter $parameter): string
{
    $parts = [];

    $type = $parameter->getType();
    if ($type !== null) {
        $parts[] = formatPhpType($type);
    }

    $name = '';
    if ($parameter->isPassedByReference()) {
        $name .= '&';
    }
    if ($parameter->isVariadic()) {
        $name .= '...';
    }
    $name .= '$' . $parameter->getName();
    $parts[] = $name;

    if (!$parameter->isVariadic() && $parameter->isOptional()) {
        try {
            if ($parameter->isDefaultValueConstant()) {
                $parts[count($parts) - 1] .= ' = ' . $parameter->getDefaultValueConstantName();
            } elseif ($parameter->isDefaultValueAvailable()) {
                $parts[count($parts) - 1] .= ' = ' . renderPhpValue($parameter->getDefaultValue());
            } else {
                $parts[count($parts) - 1] .= ' = null';
            }
        } catch (ReflectionException) {
            $parts[count($parts) - 1] .= ' = null';
        }
    }

    return implode(' ', $parts);
}

function getEffectiveReturnType(ReflectionFunctionAbstract $function): ReflectionType|null
{
    if ($function->hasReturnType()) {
        return $function->getReturnType();
    }

    if (method_exists($function, 'hasTentativeReturnType') && $function->hasTentativeReturnType()) {
        return $function->getTentativeReturnType();
    }

    return null;
}

function formatPhpType(ReflectionType $type): string
{
    if ($type instanceof ReflectionNamedType) {
        $name = formatNamedTypeName($type->getName());
        if ($type->allowsNull() && !in_array(strtolower($type->getName()), ['mixed', 'null'], true)) {
            return '?' . $name;
        }

        return $name;
    }

    if ($type instanceof ReflectionUnionType) {
        $parts = array_map(formatPhpType(...), $type->getTypes());
        return implode('|', $parts);
    }

    if ($type instanceof ReflectionIntersectionType) {
        $parts = array_map(formatPhpType(...), $type->getTypes());
        return implode('&', $parts);
    }

    return 'mixed';
}

function formatNamedTypeName(string $name): string
{
    $lower = strtolower($name);
    if (in_array($lower, [
        'array', 'bool', 'callable', 'false', 'float', 'int', 'iterable', 'mixed',
        'never', 'null', 'object', 'parent', 'self', 'static', 'string', 'true', 'void',
    ], true)) {
        return $lower === 'false' || $lower === 'true' ? $lower : $name;
    }

    return qualifyTypeName($name);
}

function qualifyTypeName(string $name): string
{
    if (str_starts_with($name, '\\')) {
        return $name;
    }

    return '\\' . $name;
}

/**
 * @param mixed $value
 */
function renderPhpValue(mixed $value): string
{
    if (is_string($value)) {
        return var_export($value, true);
    }

    if (is_int($value) || is_bool($value) || $value === null) {
        return var_export($value, true);
    }

    if (is_float($value)) {
        if (is_nan($value)) {
            return 'NAN';
        }
        if ($value === INF) {
            return 'INF';
        }
        if ($value === -INF) {
            return '-INF';
        }

        $rendered = var_export($value, true);
        return str_contains($rendered, '.') ? $rendered : $rendered . '.0';
    }

    if (is_array($value)) {
        $parts = [];
        $expectedKey = 0;
        foreach ($value as $key => $item) {
            $renderedItem = renderPhpValue($item);
            if (is_int($key) && $key === $expectedKey) {
                $parts[] = $renderedItem;
                $expectedKey++;
                continue;
            }

            $parts[] = renderPhpValue($key) . ' => ' . $renderedItem;
        }

        return '[' . implode(', ', $parts) . ']';
    }

    return 'null';
}

function indentBlock(string $block): string
{
    return implode("\n", array_map(static fn(string $line): string => '    ' . $line, explode("\n", $block)));
}

/**
 * Runs $code with each PHP build in $commands (separated by ';', each split on
 * spaces and run as an argv array, without the parent's ini settings) and
 * returns the JSON each prints, oldest version first.
 *
 * @return list<array<string, mixed>>
 */
function runPhpDumps(string $commands, string $code): array
{
    $env = getenv();
    unset($env['PHP_INI_SCAN_DIR'], $env['PHPRC']);
    $dumps = [];
    foreach (array_filter(array_map('trim', explode(';', $commands))) as $command) {
        $argv = array_merge(array_values(array_filter(explode(' ', $command), 'strlen')), ['-r', $code]);
        $process = proc_open($argv, [1 => ['pipe', 'w'], 2 => STDERR], $pipes, null, $env);
        if (!is_resource($process)) {
            fwrite(STDERR, "Failed to run: {$command}\n");
            exit(1);
        }
        $json = stream_get_contents($pipes[1]);
        fclose($pipes[1]);
        if (proc_close($process) !== 0) {
            fwrite(STDERR, "PHP failed: {$command}\n");
            exit(1);
        }
        try {
            $dumps[] = json_decode((string) $json, true, 512, JSON_THROW_ON_ERROR);
        } catch (JsonException $e) {
            fwrite(STDERR, "Unreadable output from {$command}: {$e->getMessage()}\n");
            exit(1);
        }
    }
    usort($dumps, static fn(array $a, array $b): int => version_compare($a['version'], $b['version']));
    return $dumps;
}

/**
 * The PHP type name of a gettype() result.
 */
function phpTypeName(string $gettype): string
{
    return match ($gettype) {
        'integer' => 'int',
        'double' => 'float',
        'boolean' => 'bool',
        'NULL' => 'null',
        default => $gettype,
    };
}

function writeOutput(string $output, ?string $path): void
{
    if ($path === null) {
        fwrite(STDOUT, $output);
        return;
    }

    $directory = dirname($path);
    if (!is_dir($directory)) {
        fwrite(STDERR, "Output directory does not exist: {$directory}\n");
        exit(1);
    }

    $result = file_put_contents($path, $output);
    if ($result === false) {
        fwrite(STDERR, "Failed to write output: {$path}\n");
        exit(1);
    }
}

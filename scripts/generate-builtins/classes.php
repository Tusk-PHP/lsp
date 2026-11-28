<?php

declare(strict_types=1);

/**
 * PHP code, run in each build, that starts its dump: the version, whether the
 * build is thread-safe (ZTS), the loaded extensions, and the static ones,
 * which a run without any ini file (-n) still loads.
 */
function dumpHeaderCode(): string
{
    return <<<'PHP'
$static = json_decode((string) shell_exec(escapeshellarg(PHP_BINARY) . ' -n -r ' . escapeshellarg('echo json_encode(array_map("strtolower", get_loaded_extensions()));')), true);
if (!is_array($static)) {
    fwrite(STDERR, "Can't list the static extensions of " . PHP_BINARY . "\n");
    exit(1);
}
$out = ['version' => PHP_MAJOR_VERSION . '.' . PHP_MINOR_VERSION, 'zts' => (bool) PHP_ZTS, 'extensions' => array_map('strtolower', get_loaded_extensions()), 'static' => $static];

PHP;
}

function classLikeDumpCode(): string
{
    return dumpHeaderCode() . <<<'PHP'
$type = function ($t) use (&$type) {
    if ($t === null) {
        return '';
    }
    if ($t instanceof ReflectionNamedType) {
        $name = $t->getName();
        return ($t->allowsNull() && $name !== 'mixed' && $name !== 'null' ? '?' : '') . $name;
    }
    return (string) $t;
};
$params = function ($f) use ($type) {
    $out = [];
    foreach ($f->getParameters() as $p) {
        $default = null;
        try {
            if ($p->isDefaultValueAvailable()) {
                $default = $p->isDefaultValueConstant() ? ['constant' => $p->getDefaultValueConstantName()] : ['value' => $p->getDefaultValue()];
            }
        } catch (ReflectionException $e) {
        }
        $out[] = ['$' . $p->getName(), $type($p->getType()), $default, $p->isVariadic(), $p->isPassedByReference()];
    }
    return $out;
};
$visibility = function ($m) {
    return $m->isProtected() ? 'protected' : 'public';
};
$names = array_merge(get_declared_classes(), get_declared_interfaces(), get_declared_traits());
$out['classes'] = [];
foreach ($names as $name) {
    $c = new ReflectionClass($name);
    if (!$c->isInternal()) {
        continue;
    }
    $kind = $c->isInterface() ? 'interface' : ($c->isTrait() ? 'trait' : (method_exists($c, 'isEnum') && $c->isEnum() ? 'enum' : 'class'));
    $parent = $c->getParentClass();
    $entry = [
        'name' => $c->getName(),
        'kind' => $kind,
        'extension' => strtolower((string) $c->getExtensionName()),
        'abstract' => $c->isAbstract() && $kind === 'class',
        'final' => $c->isFinal(),
        'extends' => $parent ? $parent->getName() : '',
        'implements' => $c->getInterfaceNames(),
        'methods' => [],
        'properties' => [],
        'constants' => [],
        'cases' => [],
    ];
    foreach ($c->getMethods() as $m) {
        if ($m->isPrivate() || $m->getDeclaringClass()->getName() !== $c->getName()) {
            continue;
        }
        $return = $m->hasReturnType() ? $m->getReturnType() : (method_exists($m, 'hasTentativeReturnType') && $m->hasTentativeReturnType() ? $m->getTentativeReturnType() : null);
        $entry['methods'][$m->getName()] = [$visibility($m), $m->isStatic(), $m->isAbstract(), $m->isFinal(), $params($m), $type($return)];
    }
    foreach ($c->getProperties() as $p) {
        if ($p->isPrivate() || $p->getDeclaringClass()->getName() !== $c->getName()) {
            continue;
        }
        $entry['properties']['$' . $p->getName()] = [$visibility($p), $p->isStatic(), method_exists($p, 'isReadOnly') && $p->isReadOnly(), method_exists($p, 'getType') ? $type($p->getType()) : ''];
    }
    $isCase = function ($rc) {
        return method_exists($rc, 'isEnumCase') && $rc->isEnumCase();
    };
    foreach ($c->getReflectionConstants() as $k) {
        if ($k->isPrivate() || $k->getDeclaringClass()->getName() !== $c->getName()) {
            continue;
        }
        if ($isCase($k)) {
            $entry['cases'][$k->getName()] = true;
            continue;
        }
        $v = $k->getValue();
        $entry['constants'][$k->getName()] = [$visibility($k), gettype($v), is_scalar($v) || is_array($v) || $v === null ? ['value' => $v] : null];
    }
    $out['classes'][$c->getName()] = $entry;
}
echo json_encode($out, JSON_INVALID_UTF8_SUBSTITUTE);
PHP;
}

/**
 * When builds disagree about a class or member, which versions it is in.
 *
 * Each build lists the extensions it loaded. A build without something proves
 * it absent only when that build loaded the extensions that provide it, or
 * lacked just extensions that had not come to PHP yet: ones compiled into the
 * builds of the version that brought them and in every build from then on, as
 * random was with 8.2. An extension first seen as a shared one (opcache in the
 * 8.1 images) may simply not have been enabled before. A build that
 * lacked a provider, such as a default build without pdo_mysql for
 * PDO::MYSQL_ATTR_*, proves nothing.
 */
final class BuildVersions
{
    /** @var list<string> */
    public readonly array $versions;
    /** @var list<array<string, true>> */
    private array $extensions = [];
    /** @var array<string, true> */
    private array $arrived = [];

    /**
     * @param list<array{version:string, extensions:list<string>, static:list<string>}> $dumps sorted by version
     */
    public function __construct(private readonly array $dumps)
    {
        $first = [];
        foreach ($dumps as $i => $dump) {
            $this->extensions[$i] = array_fill_keys($dump['extensions'], true);
            foreach ($dump['extensions'] as $extension) {
                $first[$extension] ??= $dump['version'];
            }
        }
        foreach ($first as $extension => $since) {
            $everywhere = true;
            foreach ($dumps as $i => $dump) {
                $at = version_compare($dump['version'], $since);
                if ($at >= 0 && !isset($this->extensions[$i][$extension]) || $at === 0 && !in_array($extension, $dump['static'], true)) {
                    $everywhere = false;
                    break;
                }
            }
            if ($everywhere) {
                $this->arrived[$extension] = true;
            }
        }
        $this->versions = array_values(array_unique(array_map(static fn(array $d): string => $d['version'], $dumps)));
    }

    public function version(int $build): string
    {
        return $this->dumps[$build]['version'];
    }

    /**
     * The version that added something present in the builds $in, '' when
     * it is as old as the oldest build or no build can show its absence.
     * Of the builds in $candidates (all builds when null), one before its
     * first version that lacked it while having its providers shows it
     * absent; it was added in the version after the newest such build.
     *
     * @param non-empty-list<int> $in
     * @param array<string, true> $providers
     * @param ?list<int> $candidates
     */
    public function since(array $in, array $providers, ?array $candidates = null): string
    {
        $first = $this->version($in[0]);
        $proof = null;
        foreach ($candidates ?? array_keys($this->dumps) as $build) {
            if (version_compare($this->version($build), $first, '>=')) {
                break;
            }
            if (array_diff_key($providers, $this->extensions[$build], $this->arrived) === []) {
                $proof = $this->version($build);
            }
        }
        return $proof === null ? '' : $this->versionAfter($proof);
    }

    /**
     * The version that removed something present in the builds $in: the
     * first build of $candidates after its last version that lacked it
     * while having its providers, '' when there is none.
     *
     * @param non-empty-list<int> $in
     * @param array<string, true> $providers
     * @param list<int> $candidates
     */
    public function removed(array $in, array $providers, array $candidates): string
    {
        $last = $this->version($in[array_key_last($in)]);
        foreach ($candidates as $build) {
            if (version_compare($this->version($build), $last, '>') && array_diff_key($providers, $this->extensions[$build]) === []) {
                return $this->version($build);
            }
        }
        return '';
    }

    /**
     * The extensions a member present in the builds $in needs, when
     * reflection doesn't say: its class's extension, and those every build
     * in $in loaded that a build of $witnesses without it lacks, from its
     * first version on. A driver (pdo_mysql for PDO::MYSQL_ATTR_*) is one of
     * them; an extension every build of those versions loads (opcache) is
     * not.
     *
     * @param non-empty-list<int> $in
     * @param list<int> $witnesses
     * @return array<string, true>
     */
    public function memberProviders(array $in, string $classExtension, array $witnesses): array
    {
        $shared = array_intersect_key(...array_map(fn(int $build): array => $this->extensions[$build], $in));
        $first = $this->version($in[0]);
        $has = array_fill_keys($in, true);
        $providers = [$classExtension => true];
        foreach ($witnesses as $build) {
            if (!isset($has[$build]) && version_compare($this->version($build), $first, '>=')) {
                $providers += array_diff_key($shared, $this->extensions[$build]);
            }
        }
        return $providers;
    }

    private function versionAfter(string $version): string
    {
        return $this->versions[array_search($version, $this->versions, true) + 1];
    }
}

/**
 * The interfaces a class implements in every version that has it, and the
 * ones it gained or lost, with the versions that did (SplFixedArray
 * implements Iterator in 7.4, IteratorAggregate from 8.0). A parent that
 * changes between builds can't be written to the table and stops the
 * generator.
 *
 * @param list<int> $builds the builds that have the class, sorted by version
 * @param array<int, array{extends:string, implements:list<string>}> $classIn the class in each build
 * @return array{0:list<string>, 1:list<array{0:string, 1:string, 2:string}>}
 */
function classRelations(string $name, array $builds, array $classIn, BuildVersions $versions): array
{
    $extends = array_unique(array_map(static fn(int $b): string => $classIn[$b]['extends'], $builds));
    if (count($extends) > 1) {
        fwrite(STDERR, "The parent of {$name} differs between builds: " . implode(', ', $extends) . "\n");
        exit(1);
    }
    $classVersions = [];
    $held = [];
    foreach (array_reverse($builds) as $build) {
        $version = $versions->version($build);
        $classVersions[$version] = true;
        foreach ($classIn[$build]['implements'] as $interface) {
            $held[$interface][$version] = true;
        }
    }
    $classVersions = array_reverse(array_keys($classVersions));
    $always = [];
    $changed = [];
    foreach ($held as $interface => $in) {
        $run = array_values(array_intersect($classVersions, array_keys($in)));
        $start = array_search($run[0], $classVersions, true);
        if ($run !== array_slice($classVersions, $start, count($run))) {
            fwrite(STDERR, "{$name} implements {$interface} in versions " . implode(', ', $run) . ", which are not consecutive\n");
            exit(1);
        }
        if (count($run) === count($classVersions)) {
            $always[] = (string) $interface;
            continue;
        }
        $changed[] = [(string) $interface, $start === 0 ? '' : $run[0], $classVersions[$start + count($run)] ?? ''];
    }
    return [$always, $changed];
}

/**
 * A member is dated only by builds that have its class. Properties are dated
 * only by builds whose class declares some: before 8.1, DOM, SOAP and SNMP
 * classes handled their properties internally and declared none to
 * reflection.
 *
 * @param list<array{version:string, extensions:list<string>, classes:array<string, array>}> $defaultDumps
 * @param list<array{version:string, extensions:list<string>, classes:array<string, array>}> $extensionDumps
 * @param list<array{version:string, extensions:list<string>, classes:array<string, array>}> $peclDumps
 */
function generateGoClassLikes(array $defaultDumps, array $extensionDumps, array $peclDumps, string $extensionStubsDir): string
{
    $defaultExtensions = loadedExtensions($defaultDumps);
    $phpSourceExtensions = loadedExtensions(array_merge($defaultDumps, $extensionDumps));
    $stubClasses = extensionStubClassLikes($extensionStubsDir, $defaultExtensions);
    $stubExtensions = array_flip(array_column($stubClasses, 1));
    $peclExtensions = array_diff_key(loadedExtensions($peclDumps), $phpSourceExtensions) + $stubExtensions;
    $dumps = array_merge($defaultDumps, $extensionDumps, $peclDumps);
    usort($dumps, static fn(array $a, array $b): int => version_compare($a['version'], $b['version']));

    $versions = new BuildVersions($dumps);
    $latest = [];
    $classIn = [];
    $memberIn = [];
    foreach ($dumps as $build => $dump) {
        foreach ($dump['classes'] as $name => $class) {
            if (isset($stubExtensions[$class['extension']])) {
                continue;
            }
            $classIn[$name][$build] = $class;
            $merged = $class;
            foreach (['methods', 'properties', 'constants', 'cases'] as $group) {
                $merged[$group] = $class[$group] + ($latest[$name][$group] ?? []);
                foreach (array_keys($class[$group]) as $member) {
                    $memberIn[$name][$group][$member][] = $build;
                }
            }
            $latest[$name] = $merged;
        }
    }

    foreach ($stubClasses as $name => [$kind, $extension]) {
        $latest[$name] ??= ['kind' => $kind, 'extension' => $extension, 'abstract' => false, 'final' => false, 'extends' => '', 'implements' => [],
            'methods' => [], 'properties' => [], 'constants' => [], 'cases' => []];
    }
    ksort($latest, SORT_STRING);

    $lines = [];
    foreach ($latest as $name => $class) {
        $byBuild = $classIn[$name] ?? [];
        $builds = array_keys($byBuild);
        $witnesses = [
            'methods' => $builds,
            'properties' => array_keys(array_filter($byBuild, static fn(array $c): bool => $c['properties'] !== [])),
            'constants' => $builds,
            'cases' => $builds,
        ];
        $member = static function (string $group, string $member) use ($memberIn, $name, $class, $witnesses, $versions): array {
            $in = $memberIn[$name][$group][$member];
            $providers = $versions->memberProviders($in, $class['extension'], $witnesses[$group]);
            return [
                'Since' => goStringOrNull($versions->since($in, $providers, $witnesses[$group])),
                'Removed' => goStringOrNull($versions->removed($in, $providers, $witnesses[$group])),
            ];
        };
        [$implements, $versionedImplements] = classRelations($name, $builds, $byBuild, $versions);
        $members = [];
        foreach (sortedKeys($class['methods']) as $method) {
            [$visibility, $static, $abstract, $final, $params, $return] = $class['methods'][$method];
            $members[] = renderGoStruct([
                'Name' => renderGoString($method),
                'Kind' => 'KindMethod',
                'Visibility' => renderGoString($visibility),
                'IsStatic' => goTrueOrNull($static),
                'IsAbstract' => goTrueOrNull($abstract),
                'IsFinal' => goTrueOrNull($final),
                'ReturnType' => goStringOrNull($return),
                'Params' => $params === [] ? null : '[]ParamInfo{' . implode(', ', array_map('renderGoParam', $params)) . '}',
                ...$member('methods', $method),
            ]);
        }
        foreach (sortedKeys($class['properties']) as $property) {
            [$visibility, $static, $readonly, $type] = $class['properties'][$property];
            $members[] = renderGoStruct([
                'Name' => renderGoString($property),
                'Kind' => 'KindProperty',
                'Visibility' => renderGoString($visibility),
                'IsStatic' => goTrueOrNull($static),
                'IsReadonly' => goTrueOrNull($readonly),
                'Type' => goStringOrNull($type),
                ...$member('properties', $property),
            ]);
        }
        foreach (sortedKeys($class['constants']) as $constant) {
            [$visibility, $type, $value] = $class['constants'][$constant];
            $members[] = renderGoStruct([
                'Name' => renderGoString($constant),
                'Kind' => 'KindConstant',
                'Visibility' => renderGoString($visibility),
                'Type' => renderGoString(phpTypeName($type)),
                'Value' => $value === null ? null : renderGoString(renderPhpValue($value['value'])),
                ...$member('constants', $constant),
            ]);
        }
        foreach (sortedKeys($class['cases']) as $case) {
            $members[] = renderGoStruct(['Name' => renderGoString($case), 'Kind' => 'KindEnumCase', ...$member('cases', $case)]);
        }
        $lines[] = "\t" . renderGoStruct([
            'Name' => renderGoString($name),
            'Kind' => match ($class['kind']) { 'interface' => 'KindInterface', 'trait' => 'KindTrait', 'enum' => 'KindEnum', default => 'KindClass' },
            'IsAbstract' => goTrueOrNull($class['abstract']),
            'IsFinal' => goTrueOrNull($class['final']),
            'Extends' => goStringOrNull($class['extends']),
            'Implements' => $implements === [] ? null : '[]string{' . implode(', ', array_map('renderGoString', $implements)) . '}',
            'VersionedImplements' => $versionedImplements === [] ? null : '[]builtinRelation{' . implode(', ', array_map(
                static fn(array $r): string => renderGoStruct(['Name' => renderGoString($r[0]), 'Since' => goStringOrNull($r[1]), 'Removed' => goStringOrNull($r[2])]),
                $versionedImplements
            )) . '}',
            'Since' => $builds === [] ? null : goStringOrNull($versions->since($builds, [$class['extension'] => true])),
            'Extension' => goStringOrNull(isset($peclExtensions[$class['extension']]) ? $class['extension'] : ''),
            'Members' => $members === [] ? null : "[]builtinMember{\n" . implode('', array_map(static fn(string $m): string => "\t\t" . $m . ",\n", $members)) . "\t}",
        ]) . ',';
    }

    $header = implode(', ', $versions->versions);

    return implode("\n", [
        'package symbols',
        '',
        '// Code generated from PHP runtime builtin symbols; DO NOT EDIT.',
        "// Generated by scripts/generate-builtins.php classes from reflection of PHP {$header}.",
        '',
        'var generatedBuiltinClassLikes = []builtinClassLike{',
        ...$lines,
        '}',
        '',
    ]);
}

/**
 * @param list<array{version:string, extensions:list<string>, classes:array<string, array>}> $dumps
 * @return array<string, true>
 */
function loadedExtensions(array $dumps): array
{
    $out = [];
    foreach ($dumps as $dump) {
        foreach ($dump['extensions'] as $extension) {
            $out[$extension] = true;
        }
    }
    return $out;
}

/**
 * The class-likes the embedded stubs of an optional extension declare, each
 * needing that extension: internal/stubs/php/extensions/<extension>/. The
 * stubs are those classes' source; a build's reflection of them is not used.
 *
 * @param array<string, true> $defaultExtensions
 * @return array<string, array{0:string, 1:string}> kind and extension by name
 */
function extensionStubClassLikes(string $dir, array $defaultExtensions): array
{
    $out = [];
    foreach (extensionStubSources($dir, $defaultExtensions) as [$extension, $source]) {
        foreach (declaredClassLikes($source) as $name => $kind) {
            $out[$name] = [$kind, $extension];
        }
    }
    return $out;
}

/**
 * The PHP files under internal/stubs/php/extensions/<extension>/, in path
 * order, with their extension, leaving out extensions of the default builds.
 *
 * @param array<string, true> $defaultExtensions
 * @return list<array{0:string, 1:string}>
 */
function extensionStubSources(string $dir, array $defaultExtensions): array
{
    $files = new RecursiveIteratorIterator(new RecursiveDirectoryIterator($dir, FilesystemIterator::SKIP_DOTS | FilesystemIterator::UNIX_PATHS));
    $paths = [];
    foreach ($files as $info) {
        if ($info->getExtension() === 'php') {
            $paths[] = $info->getPathname();
        }
    }
    sort($paths, SORT_STRING);
    $out = [];
    foreach ($paths as $file) {
        $extension = strtolower(explode('/', substr($file, strlen($dir) + 1))[0]);
        if (isset($defaultExtensions[$extension])) {
            continue;
        }
        $source = file_get_contents($file);
        if ($source === false) {
            fwrite(STDERR, "Can't read {$file}\n");
            exit(1);
        }
        $out[] = [$extension, $source];
    }
    return $out;
}

function declaredClassLikes(string $source): array
{
    $tokens = array_values(array_filter(
        token_get_all($source),
        static fn($t): bool => !is_array($t) || !in_array($t[0], [T_WHITESPACE, T_COMMENT, T_DOC_COMMENT], true)
    ));
    $namespace = '';
    $names = [];
    foreach ($tokens as $i => $token) {
        if (!is_array($token)) {
            continue;
        }
        if ($token[0] === T_NAMESPACE) {
            $next = $tokens[$i + 1] ?? null;
            $namespace = is_array($next) && in_array($next[0], [T_STRING, T_NAME_QUALIFIED], true) ? $next[1] . '\\' : '';
            continue;
        }
        if (!in_array($token[0], [T_CLASS, T_INTERFACE, T_TRAIT, T_ENUM], true)) {
            continue;
        }
        $previous = $tokens[$i - 1] ?? null;
        if (is_array($previous) && in_array($previous[0], [T_DOUBLE_COLON, T_NEW], true)) {
            continue;
        }
        $next = $tokens[$i + 1] ?? null;
        if (is_array($next) && $next[0] === T_STRING) {
            $names[$namespace . $next[1]] = match ($token[0]) { T_INTERFACE => 'interface', T_TRAIT => 'trait', T_ENUM => 'enum', default => 'class' };
        }
    }
    return $names;
}

/**
 * @param array<string, ?string> $fields Go field values, null for a field left at its zero value
 */
function renderGoStruct(array $fields): string
{
    $parts = [];
    foreach ($fields as $field => $value) {
        if ($value !== null) {
            $parts[] = $field . ': ' . $value;
        }
    }
    return '{' . implode(', ', $parts) . '}';
}

function goTrueOrNull(bool $value): ?string
{
    return $value ? 'true' : null;
}

function goStringOrNull(string $value): ?string
{
    return $value === '' ? null : renderGoString($value);
}

/**
 * @param array{0:string, 1:string, 2:?array{constant?:string, value?:mixed}, 3:bool, 4:bool} $param
 */
function renderGoParam(array $param): string
{
    [$name, $type, $default, $variadic, $byRef] = $param;
    return renderGoStruct([
        'Name' => renderGoString($name),
        'Type' => goStringOrNull($type),
        'DefaultValue' => match (true) {
            $default === null => null,
            isset($default['constant']) => renderGoString($default['constant']),
            default => renderGoString(renderPhpValue($default['value'])),
        },
        'IsVariadic' => goTrueOrNull($variadic),
        'IsReference' => goTrueOrNull($byRef),
    ]);
}

function sortedKeys(array $map): array
{
    $keys = array_map('strval', array_keys($map));
    sort($keys, SORT_STRING);
    return $keys;
}

return [
    'name' => 'classes',
    'usage' => 'php scripts/generate-builtins.php classes --php=COMMANDS [--extensions-php=COMMANDS] [--pecl-php=COMMANDS] [--output=PATH]',
    'help' => '  classes       Run each PHP given with --php and emit the Go table of PHP\'s
                built-in classes, interfaces, traits and enums with their
                public and protected methods, properties and constants. A
                class has the members any build gives it (an extension such
                as pdo_mysql adds constants to PDO); each signature comes from
                the newest PHP that has it. Each class, member and interface
                carries the version that added it and the one that removed
                it, when a build that had the extensions providing it shows
                it missing; pass builds with extensions for every version to
                date more of them.
                Output defaults to internal/symbols/builtin_classes_generated.go.
',
    'run' => static function (array $options): void {
        if ($options['php'] === '') {
            fwrite(STDERR, "Classes mode needs PHP's default builds (--php).\n");
            exit(1);
        }
        writeOutput(
            generateGoClassLikes(
                runPhpDumps($options['php'], classLikeDumpCode()),
                runPhpDumps($options['extensions-php'], classLikeDumpCode()),
                runPhpDumps($options['pecl-php'], classLikeDumpCode()),
                dirname(__DIR__, 2) . '/internal/stubs/php/extensions'
            ),
            $options['output'] ?? dirname(__DIR__, 2) . '/internal/symbols/builtin_classes_generated.go'
        );
    },
];

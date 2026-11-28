<?php

declare(strict_types=1);

function classLikeDumpCode(): string
{
    return <<<'PHP'
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
$out = ['version' => PHP_MAJOR_VERSION . '.' . PHP_MINOR_VERSION, 'extensions' => array_map('strtolower', get_loaded_extensions()), 'classes' => []];
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
 * lacked just extensions that had not come to PHP yet (ones every build has
 * from the version that brought them, as random did with 8.2). A build that
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
     * @param list<array{version:string, extensions:list<string>}> $dumps sorted by version
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
                if (version_compare($dump['version'], $since, '>=') && !isset($this->extensions[$i][$extension])) {
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
 * @param list<array{version:string, extensions:list<string>, classes:array<string, array>}> $dumps
 */
function generateGoClassLikes(array $dumps): string
{
    $versions = new BuildVersions($dumps);
    $latest = [];
    $classIn = [];
    $memberIn = [];
    foreach ($dumps as $build => $dump) {
        foreach ($dump['classes'] as $name => $class) {
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
    ksort($latest, SORT_STRING);

    $lines = [];
    foreach ($latest as $name => $class) {
        $builds = array_keys($classIn[$name]);
        $witnesses = [
            'methods' => $builds,
            'properties' => array_keys(array_filter($classIn[$name], static fn(array $c): bool => $c['properties'] !== [])),
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
        [$implements, $versionedImplements] = classRelations($name, $builds, $classIn[$name], $versions);
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
            'Since' => goStringOrNull($versions->since($builds, [$class['extension'] => true])),
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
    'usage' => 'php scripts/generate-builtins.php classes --php=COMMANDS [--output=PATH]',
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
            generateGoClassLikes(runPhpDumps($options['php'], classLikeDumpCode())),
            $options['output'] ?? dirname(__DIR__, 2) . '/internal/symbols/builtin_classes_generated.go'
        );
    },
];

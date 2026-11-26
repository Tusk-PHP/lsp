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
 * The version that added something first seen in $first: that version, when an
 * earlier build had the extension it belongs to without it. Something of an
 * extension no earlier build had is taken to be as old as the baseline.
 */
function addedIn(string $first, string $extension, array $dumps): string
{
    if ($first === $dumps[0]['version']) {
        return '';
    }
    foreach ($dumps as $dump) {
        if (version_compare($dump['version'], $first, '>=')) {
            break;
        }
        if ($extension === '' || in_array($extension, $dump['extensions'], true)) {
            return $first;
        }
    }
    return '';
}

/**
 * @param list<array{version:string, extensions:list<string>, classes:array<string, array>}> $dumps
 */
function generateGoClassLikes(array $dumps): string
{
    $first = [];
    $latest = [];
    $memberFirst = [];
    $memberLast = [];
    $classVersions = [];
    foreach ($dumps as $dump) {
        foreach ($dump['classes'] as $name => $class) {
            $first[$name] ??= $dump['version'];
            $classVersions[$name][$dump['version']] = true;
            $merged = $class;
            foreach (['methods', 'properties', 'constants', 'cases'] as $group) {
                $merged[$group] = $class[$group] + ($latest[$name][$group] ?? []);
                foreach (array_keys($class[$group]) as $member) {
                    $memberFirst[$name][$group][$member] ??= $dump['version'];
                    $memberLast[$name][$group][$member] = $dump['version'];
                }
            }
            $latest[$name] = $merged;
        }
    }
    ksort($latest, SORT_STRING);

    $lines = [];
    foreach ($latest as $name => $class) {
        $since = static function (string $group, string $member) use ($memberFirst, $name, $first, $class, $dumps): ?string {
            $at = $memberFirst[$name][$group][$member];
            return $at === $first[$name] ? null : goStringOrNull(addedIn($at, $class['extension'], $dumps));
        };
        $removed = static function (string $group, string $member) use ($memberLast, $name, $classVersions): ?string {
            $last = $memberLast[$name][$group][$member];
            foreach (array_keys($classVersions[$name]) as $version) {
                if (version_compare((string) $version, $last, '>')) {
                    return renderGoString((string) $version);
                }
            }
            return null;
        };
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
                'Since' => $since('methods', $method),
                'Removed' => $removed('methods', $method),
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
                'Since' => $since('properties', $property),
                'Removed' => $removed('properties', $property),
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
                'Since' => $since('constants', $constant),
                'Removed' => $removed('constants', $constant),
            ]);
        }
        foreach (sortedKeys($class['cases']) as $case) {
            $members[] = renderGoStruct(['Name' => renderGoString($case), 'Kind' => 'KindEnumCase', 'Since' => $since('cases', $case), 'Removed' => $removed('cases', $case)]);
        }
        $lines[] = "\t" . renderGoStruct([
            'Name' => renderGoString($name),
            'Kind' => match ($class['kind']) { 'interface' => 'KindInterface', 'trait' => 'KindTrait', 'enum' => 'KindEnum', default => 'KindClass' },
            'IsAbstract' => goTrueOrNull($class['abstract']),
            'IsFinal' => goTrueOrNull($class['final']),
            'Extends' => goStringOrNull($class['extends']),
            'Implements' => $class['implements'] === [] ? null : '[]string{' . implode(', ', array_map('renderGoString', $class['implements'])) . '}',
            'Members' => $members === [] ? null : "[]builtinMember{\n" . implode('', array_map(static fn(string $m): string => "\t\t" . $m . ",\n", $members)) . "\t}",
        ]) . ',';
    }

    $versions = implode(', ', array_unique(array_map(static fn(array $d): string => $d['version'], $dumps)));

    return implode("\n", [
        'package symbols',
        '',
        '// Code generated from PHP runtime builtin symbols; DO NOT EDIT.',
        "// Generated by scripts/generate-builtins.php classes from reflection of PHP {$versions}.",
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
                public and protected methods, properties and constants, the
                version that added each member and the one that removed it.
                A class has the members any build gives it (an extension such
                as pdo_mysql adds constants to PDO); each signature comes from
                the newest PHP that has it.
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

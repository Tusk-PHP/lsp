<?php

declare(strict_types=1);

/**
 * Functions and methods (lower-case Class::method) whose PHPStan function map
 * types were checked against PHP and found wrong for some version, so they
 * keep their native signature:
 * strcmp() and its kin return the byte difference (-2 for "a" and "c"), not
 * int<-1, 1>; array_unshift() returns 0 for an empty array and no values;
 * proc_get_status() gained "cached" in 8.3 and stream_context_get_params()
 * has "notification" only once one is set; password_get_info()'s algo is a
 * string since 7.4; str_ireplace() kept nested arrays before 8.0;
 * pg_fetch_all() returns rows keyed by column name; array_combine() keys,
 * preg_replace_callback() matches (with flags), round() modes, unserialize()
 * options (max_depth since 8.1), ob_list_handlers() (never false) and
 * opcache_get_configuration() directives are narrower than what PHP accepts
 * or returns. PDOStatement::rowCount() passes on what the driver reports,
 * which is -1 from ODBC when it doesn't know.
 *
 * @return array<string, true>
 */
function unverifiedPhpstanSignatures(): array
{
    return array_fill_keys([
        'array_combine',
        'array_unshift',
        'ob_list_handlers',
        'opcache_get_configuration',
        'password_get_info',
        'pdostatement::rowcount',
        'pg_fetch_all',
        'preg_replace_callback',
        'proc_get_status',
        'round',
        'str_ireplace',
        'stream_context_get_params',
        'strcasecmp',
        'strcmp',
        'strcoll',
        'strncasecmp',
        'strncmp',
        'substr_compare',
        'unserialize',
    ], true);
}

/**
 * PHPStan's function map for each version, fetched from phpstan-src at the
 * tag $ref: resources/functionMap.php with every functionMap_phpXYdelta.php
 * up to that version applied, as PHPStan applies them (the delta's old
 * entries removed, then its new ones added). Keys are lower-case names; an
 * entry is the return type followed by the parameter types keyed by name.
 *
 * @param list<string> $versions
 * @return array<string, array<string, array<int|string, string>>> by version
 */
function phpstanFunctionMaps(string $ref, array $versions): array
{
    $base = 'https://raw.githubusercontent.com/phpstan/phpstan-src/' . rawurlencode($ref) . '/resources/';
    $map = array_change_key_case(readPhpReturnedArray(fetchPhpstanResource($base . 'functionMap.php', false), $base . 'functionMap.php'), CASE_LOWER);
    $maps = [];
    foreach ($versions as $version) {
        $url = $base . 'functionMap_php' . str_replace('.', '', $version) . 'delta.php';
        $source = fetchPhpstanResource($url, true);
        if ($source !== null) {
            $changes = readPhpReturnedArray($source, $url);
            foreach (array_keys($changes['old']) as $name) {
                unset($map[strtolower((string) $name)]);
            }
            foreach ($changes['new'] as $name => $signature) {
                $map[strtolower((string) $name)] = $signature;
            }
        }
        $maps[$version] = $map;
    }
    return $maps;
}

/**
 * The body of $url, or null when it is missing (HTTP 404) and $optional.
 */
function fetchPhpstanResource(string $url, bool $optional): ?string
{
    $context = stream_context_create(['http' => ['timeout' => 60, 'ignore_errors' => true]]);
    $body = @file_get_contents($url, false, $context);
    $statusLine = http_get_last_response_headers()[0] ?? 'no response';
    $status = (int) (explode(' ', $statusLine)[1] ?? 0);
    if ($status === 404 && $optional) {
        return null;
    }
    if ($body === false || $status !== 200) {
        fwrite(STDERR, "Can't fetch {$url}: {$statusLine}\n");
        exit(1);
    }
    return $body;
}

/**
 * The array PHP source of the form `<?php return [...];` returns, read with
 * PHP's tokenizer instead of running it. Only arrays, single-quoted strings
 * and integers are accepted. $file names the source in errors.
 */
function readPhpReturnedArray(string $source, string $file): array
{
    $tokens = array_values(array_filter(
        token_get_all($source),
        static fn($t): bool => !is_array($t) || !in_array($t[0], [T_OPEN_TAG, T_WHITESPACE, T_COMMENT, T_DOC_COMMENT], true)
    ));
    $at = 0;
    $expect = static function (int|string $kind) use (&$tokens, &$at, $file): array|string {
        $token = $tokens[$at] ?? null;
        if ($token === null || (is_array($token) ? $token[0] : $token) !== $kind) {
            fwrite(STDERR, "{$file}: expected " . (is_int($kind) ? token_name($kind) : $kind) . " at token {$at}\n");
            exit(1);
        }
        $at++;
        return $token;
    };
    $value = static function () use (&$value, &$tokens, &$at, $expect, $file): array|string|int {
        $token = $tokens[$at] ?? null;
        if ($token === '[') {
            $at++;
            $out = [];
            while (($tokens[$at] ?? null) !== ']') {
                $item = $value();
                if (is_array($tokens[$at] ?? null) && $tokens[$at][0] === T_DOUBLE_ARROW) {
                    $at++;
                    if (is_array($item)) {
                        fwrite(STDERR, "{$file}: array used as a key at token {$at}\n");
                        exit(1);
                    }
                    $out[$item] = $value();
                } else {
                    $out[] = $item;
                }
                if (($tokens[$at] ?? null) !== ']') {
                    $expect(',');
                }
            }
            $at++;
            return $out;
        }
        if (is_array($token) && $token[0] === T_LNUMBER) {
            $at++;
            return (int) $token[1];
        }
        $string = $expect(T_CONSTANT_ENCAPSED_STRING)[1];
        if ($string[0] !== "'") {
            fwrite(STDERR, "{$file}: only single-quoted strings are read, got {$string}\n");
            exit(1);
        }
        return strtr(substr($string, 1, -1), ['\\\\' => '\\', "\\'" => "'"]);
    };
    $expect(T_RETURN);
    $array = $value();
    $expect(';');
    if (!is_array($array)) {
        fwrite(STDERR, "{$file} doesn't return an array\n");
        exit(1);
    }
    return $array;
}

/**
 * The kinds of value a PHPDoc or native type allows, as sorted names: int,
 * float, string, true, false, null, array, object, callable, iterable,
 * resource, mixed, void, never, static, self, and class:<lower-case name>.
 * A refinement such as list<string> or non-empty-string has the kind of what
 * it refines. Every class a type names, anywhere in it, must be one of
 * $classes. Null for what the server's type reading doesn't follow the same
 * way or can't be checked: intersections, conditional types, int masks,
 * key-of and value-of, PHPStan's own __benevolent, unknown names.
 *
 * @param array<string, true> $classes lower-case
 * @return ?list<string>
 */
function typeKinds(string $type, array $classes): ?array
{
    $tokens = typeTokens($type);
    if ($tokens === null) {
        return null;
    }
    $at = 0;
    $kinds = typeUnion($tokens, $at, $classes);
    if ($kinds === null || $at !== count($tokens)) {
        return null;
    }
    $kinds = array_values(array_unique($kinds));
    sort($kinds, SORT_STRING);
    return $kinds;
}

/**
 * @return ?list<array{0:string, 1:string}> kind (name, string, number, punctuation) and text
 */
function typeTokens(string $type): ?array
{
    $out = [];
    $length = strlen($type);
    for ($i = 0; $i < $length;) {
        $c = $type[$i];
        if ($c === ' ' || $c === "\t" || $c === "\n") {
            $i++;
            continue;
        }
        if ($c === "'" || $c === '"') {
            $end = strpos($type, $c, $i + 1);
            if ($end === false) {
                return null;
            }
            $out[] = ['string', substr($type, $i, $end - $i + 1)];
            $i = $end + 1;
            continue;
        }
        if (substr($type, $i, 3) === '...') {
            $out[] = ['punctuation', '...'];
            $i += 3;
            continue;
        }
        if (str_contains('<>{}()[]|&,:?=', $c)) {
            $out[] = ['punctuation', $c];
            $i++;
            continue;
        }
        $start = $i;
        if ($c === '-') {
            $i++;
        }
        $digits = strspn($type, '0123456789', $i);
        if ($digits > 0) {
            $i += $digits;
            if (($type[$i] ?? '') === '.') {
                $i += 1 + strspn($type, '0123456789', $i + 1);
            }
            $out[] = ['number', substr($type, $start, $i - $start)];
            continue;
        }
        $name = strspn($type, 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_\\-$', $i);
        if ($name === 0) {
            return null;
        }
        $out[] = ['name', substr($type, $i, $name)];
        $i += $name;
    }
    return $out;
}

/**
 * @param list<array{0:string, 1:string}> $tokens
 * @param array<string, true> $classes
 * @return ?list<string>
 */
function typeUnion(array $tokens, int &$at, array $classes): ?array
{
    $kinds = [];
    do {
        $part = typePostfix($tokens, $at, $classes);
        if ($part === null || typeIs($tokens, $at, '&')) {
            return null;
        }
        array_push($kinds, ...$part);
    } while (typeIs($tokens, $at, '|') && ++$at);
    return $kinds;
}

function typeIs(array $tokens, int $at, string $punctuation): bool
{
    return ($tokens[$at] ?? null) === ['punctuation', $punctuation];
}

/**
 * @return ?list<string>
 */
function typePostfix(array $tokens, int &$at, array $classes): ?array
{
    $kinds = typeAtom($tokens, $at, $classes);
    while ($kinds !== null && typeIs($tokens, $at, '[')) {
        if (!typeIs($tokens, $at + 1, ']')) {
            return null;
        }
        $at += 2;
        $kinds = ['array'];
    }
    return $kinds;
}

/**
 * @return ?list<string>
 */
function typeAtom(array $tokens, int &$at, array $classes): ?array
{
    $token = $tokens[$at] ?? null;
    if ($token === null) {
        return null;
    }
    [$kind, $text] = $token;
    $at++;
    if ($kind === 'string') {
        return ['string'];
    }
    if ($kind === 'number') {
        return [str_contains($text, '.') ? 'float' : 'int'];
    }
    if ($token === ['punctuation', '?']) {
        $inner = typePostfix($tokens, $at, $classes);
        return $inner === null ? null : [...$inner, 'null'];
    }
    if ($token === ['punctuation', '(']) {
        if (($tokens[$at][0] ?? '') === 'name' && str_starts_with($tokens[$at][1], '$')) {
            return null;
        }
        $inner = typeUnion($tokens, $at, $classes);
        return $inner !== null && typeIs($tokens, $at, ')') && ++$at ? $inner : null;
    }
    if ($kind !== 'name') {
        return null;
    }
    $name = strtolower(ltrim($text, '\\'));
    $kinds = pseudoTypeKinds()[$name] ?? (isset($classes[$name]) ? ['class:' . $name] : null);
    if ($kinds === null) {
        return null;
    }
    if (typeIs($tokens, $at, '<')) {
        $at++;
        return typeArguments($tokens, $at, $name, $classes) ? $kinds : null;
    }
    if (typeIs($tokens, $at, '{')) {
        $at++;
        return in_array($name, ['array', 'list', 'object'], true) && typeShape($tokens, $at, $classes) ? $kinds : null;
    }
    if (typeIs($tokens, $at, '(') && in_array($name, ['callable', 'closure'], true)) {
        $at++;
        return typeCallable($tokens, $at, $classes) ? $kinds : null;
    }
    return $kinds;
}

/**
 * @return array<string, list<string>>
 */
function pseudoTypeKinds(): array
{
    $string = ['string'];
    $int = ['int'];
    return [
        'array' => ['array'], 'list' => ['array'], 'non-empty-array' => ['array'], 'non-empty-list' => ['array'],
        'string' => $string, 'non-empty-string' => $string, 'non-falsy-string' => $string, 'truthy-string' => $string,
        'numeric-string' => $string, 'literal-string' => $string, 'non-empty-literal-string' => $string,
        'lowercase-string' => $string, 'non-empty-lowercase-string' => $string, 'uppercase-string' => $string,
        'non-empty-uppercase-string' => $string, 'class-string' => $string, 'interface-string' => $string,
        'trait-string' => $string, 'callable-string' => $string,
        'int' => $int, 'positive-int' => $int, 'negative-int' => $int, 'non-negative-int' => $int,
        'non-positive-int' => $int, 'non-zero-int' => $int,
        'float' => ['float'], 'bool' => ['false', 'true'], 'true' => ['true'], 'false' => ['false'], 'null' => ['null'],
        'array-key' => ['int', 'string'], 'scalar' => ['false', 'float', 'int', 'string', 'true'],
        'object' => ['object'], 'callable' => ['callable'], 'iterable' => ['iterable'], 'resource' => ['resource'],
        'mixed' => ['mixed'], 'void' => ['void'], 'never' => ['never'], 'static' => ['static'], 'self' => ['self'],
    ];
}

function typeArguments(array $tokens, int &$at, string $generic, array $classes): bool
{
    do {
        if ($generic === 'int') {
            $token = $tokens[$at] ?? null;
            if (!in_array($token, [['name', 'min'], ['name', 'max']], true) && ($token[0] ?? '') !== 'number') {
                return false;
            }
            $at++;
        } elseif (typeUnion($tokens, $at, $classes) === null) {
            return false;
        }
    } while (typeIs($tokens, $at, ',') && ++$at);
    if (!typeIs($tokens, $at, '>')) {
        return false;
    }
    $at++;
    return in_array($generic, ['array', 'list', 'non-empty-array', 'non-empty-list', 'iterable', 'int', 'class-string'], true);
}

function typeShape(array $tokens, int &$at, array $classes): bool
{
    while (!typeIs($tokens, $at, '}')) {
        $key = $tokens[$at] ?? null;
        $keyed = $key !== null && $key[0] !== 'punctuation'
            && (typeIs($tokens, $at + 1, ':') || (typeIs($tokens, $at + 1, '?') && typeIs($tokens, $at + 2, ':')));
        if ($keyed) {
            $at += typeIs($tokens, $at + 1, '?') ? 3 : 2;
        }
        if (typeUnion($tokens, $at, $classes) === null) {
            return false;
        }
        if (typeIs($tokens, $at, ',')) {
            $at++;
        } elseif (!typeIs($tokens, $at, '}')) {
            return false;
        }
    }
    $at++;
    return true;
}

function typeCallable(array $tokens, int &$at, array $classes): bool
{
    while (!typeIs($tokens, $at, ')')) {
        if (typeUnion($tokens, $at, $classes) === null) {
            return false;
        }
        foreach (['&', '...'] as $modifier) {
            if (typeIs($tokens, $at, $modifier)) {
                $at++;
            }
        }
        if (($tokens[$at][0] ?? '') === 'name' && str_starts_with($tokens[$at][1], '$')) {
            $at++;
        }
        if (typeIs($tokens, $at, '=')) {
            $at++;
        }
        if (typeIs($tokens, $at, ',')) {
            $at++;
        } elseif (!typeIs($tokens, $at, ')')) {
            return false;
        }
    }
    $at++;
    if (!typeIs($tokens, $at, ':')) {
        return true;
    }
    $at++;
    return typePostfix($tokens, $at, $classes) !== null;
}

/**
 * The PHPDoc type to declare over a native one: $doc when it allows the same
 * kinds of value as $native and says more about them (list<string> over
 * array, non-empty-string over string), and also, for $extra kinds, when it
 * allows those besides (the false a function returned on failure before PHP
 * 8 made it throw). Null when $doc adds nothing or can't be checked.
 *
 * @param array<string, true> $classes
 * @param list<string> $extra
 */
function refinedType(string $doc, string $native, array $classes, array $extra = []): ?string
{
    if ($native === '' || $native === 'mixed') {
        return null;
    }
    $docKinds = typeKinds($doc, $classes);
    $nativeKinds = typeKinds($native, $classes);
    if ($docKinds === null || $nativeKinds === null) {
        return null;
    }
    $added = array_diff($docKinds, $nativeKinds);
    if (array_diff($nativeKinds, $docKinds) !== [] || array_diff($added, $extra) !== []) {
        return null;
    }
    return normalizedTypeParts($doc) === normalizedTypeParts($native) ? null : $doc;
}

/**
 * @return list<string> the top-level members of a union, lower-case, without
 *                      spaces, sorted, with ?T read as T|null
 */
function normalizedTypeParts(string $type): array
{
    $type = strtolower(str_replace(' ', '', $type));
    if (str_starts_with($type, '?')) {
        $type = substr($type, 1) . '|null';
    }
    $parts = explode('|', $type);
    sort($parts, SORT_STRING);
    return $parts;
}

/**
 * The PHPDoc block declaring what PHPStan's signature for a function says more
 * than its native one in one build, or '' when it says nothing more.
 *
 * Parameters are matched by position, and named after $params, the
 * signature the table keeps. A build without native types (7.4 has few) is
 * checked against $reference, the native signature of the first PHP 8 build
 * with a native return type (methods got theirs, tentative, in 8.1), and its
 * parameters are left as they are, since PHP 8 turned resources into objects.
 * Before PHP 8 ($beforePhp8) its return may also add false or null, which
 * PHP 8 replaced by exceptions.
 *
 * @param array<int|string, string> $phpstan
 * @param array{0:string, 1:list<array>, 2:string} $build
 * @param array{0:string, 1:list<array>, 2:string} $reference
 * @param list<array> $params
 * @param array<string, true> $classes
 */
function phpstanDocBlock(array $phpstan, array $build, array $reference, array $params, array $classes, bool $beforePhp8): string
{
    [, $buildParams, $buildReturn] = $build;
    $extra = [];
    $fallback = $buildReturn === '';
    if ($fallback) {
        [, $buildParams, $buildReturn] = $reference;
        $extra = $beforePhp8 ? ['false', 'null'] : [];
    }
    $lines = [];
    $docParams = array_slice($phpstan, 1, null, true);
    if (!$fallback && count($docParams) === count($buildParams) && count($buildParams) === count($params)) {
        $i = 0;
        foreach ($docParams as $docName => $docType) {
            [$name, $nativeType, , $variadic, $byRef] = $buildParams[$i];
            $docName = (string) $docName;
            $sameShape = str_contains($docName, '...') === $variadic && str_starts_with($docName, '&') === $byRef
                && $params[$i][3] === $variadic && $params[$i][4] === $byRef;
            $type = $sameShape ? refinedType($docType, $nativeType, $classes) : null;
            if ($type !== null) {
                $lines[] = ' * @param ' . $type . ' ' . ($variadic ? '...' : '') . $params[$i][0];
            }
            $i++;
        }
    }
    $return = refinedType($phpstan[0], $buildReturn, $classes, $extra);
    if ($return !== null) {
        $lines[] = ' * @return ' . $return;
    }
    return $lines === [] ? '' : "/**\n" . implode("\n", $lines) . "\n */";
}

/**
 * The Go []builtinDoc of a function or method (lower-case $name, Class::method
 * for a method), or null when no version has a doc block: one entry per
 * version whose doc block differs from the previous version's, the first
 * without Since. $entries holds, by version, what that version's builds
 * reflect of it ([extension, params, return], null where none has it).
 *
 * @param array<string, ?array{0:string, 1:list<array>, 2:string}> $entries
 * @param list<array> $params
 * @param array<string, array<string, array<int|string, string>>> $maps
 * @param array<string, array<string, true>> $classes by version
 * @param array<string, true> $unverified
 */
function renderGoDocLayers(array $entries, string $name, array $params, array $maps, array $classes, array $unverified): ?string
{
    $reference = null;
    foreach ($entries as $version => $entry) {
        if ($entry !== null && $entry[2] !== '' && version_compare((string) $version, '8.0', '>=')) {
            $reference = $entry;
            break;
        }
    }
    $docs = [];
    $previous = null;
    foreach ($entries as $version => $entry) {
        if ($entry === null) {
            continue;
        }
        $phpstan = $maps[$version][$name] ?? null;
        $doc = $phpstan === null || isset($unverified[$name]) || isset($maps[$version][$name . "'1"])
            ? ''
            : phpstanDocBlock($phpstan, $entry, $reference ?? $entry, $params, $classes[$version], version_compare((string) $version, '8.0', '<'));
        if ($doc !== $previous) {
            $docs[] = renderGoStruct(['Since' => $previous === null ? null : renderGoString((string) $version), 'Doc' => goStringOrNull($doc)]);
            $previous = $doc;
        }
    }
    return $docs === [] || $docs === ['{}'] ? null : '[]builtinDoc{' . implode(', ', $docs) . '}';
}

/**
 * @param list<array{version:string, extensions:list<string>, classes:list<string>, functions:array<string, array{0:string, 1:list<array>, 2:string}>}> $defaultDumps
 * @param list<array{version:string, extensions:list<string>, classes:list<string>, functions:array<string, array{0:string, 1:list<array>, 2:string}>}> $extensionDumps
 * @param list<array{version:string, extensions:list<string>, classes:list<string>, functions:array<string, array{0:string, 1:list<array>, 2:string}>}> $peclDumps
 */
function generateGoFunctionSignatures(array $defaultDumps, array $extensionDumps, array $peclDumps, string $extensionStubsDir, string $phpstanRef): string
{
    $stubExtensions = array_flip(array_values(extensionStubFunctions($extensionStubsDir, loadedExtensions($defaultDumps))));
    $byVersion = [];
    foreach (array_merge($defaultDumps, $extensionDumps, $peclDumps) as $dump) {
        $version = $dump['version'];
        $byVersion[$version]['classes'] = ($byVersion[$version]['classes'] ?? []) + array_fill_keys($dump['classes'], true);
        $byVersion[$version]['functions'] = ($byVersion[$version]['functions'] ?? []) + $dump['functions'];
    }
    uksort($byVersion, 'version_compare');
    $versions = array_map('strval', array_keys($byVersion));
    $maps = phpstanFunctionMaps($phpstanRef, $versions);
    $unverified = unverifiedPhpstanSignatures();

    $latest = [];
    foreach ($byVersion as $builds) {
        foreach ($builds['functions'] as $name => $function) {
            if (isset($stubExtensions[$function[0]]) || isLanguageConstruct((string) $name)) {
                continue;
            }
            $latest[$name] = $function;
        }
    }
    ksort($latest, SORT_STRING);

    $lines = [];
    foreach ($latest as $name => [, $params, $return]) {
        $docs = renderGoDocLayers(array_map(
            static fn(array $builds): ?array => $builds['functions'][$name] ?? null,
            $byVersion
        ), $name, $params, $maps, array_map(static fn(array $builds): array => $builds['classes'], $byVersion), $unverified);
        $lines[] = "\t" . renderGoStruct([
            'Name' => renderGoString((string) $name),
            'Params' => $params === [] ? null : '[]ParamInfo{' . implode(', ', array_map('renderGoParam', $params)) . '}',
            'ReturnType' => goStringOrNull($return),
            'Docs' => $docs,
        ]) . ',';
    }

    $buildVersions = implode(', ', $versions);

    return implode("\n", [
        'package symbols',
        '',
        '// Code generated from PHP runtime builtin symbols; DO NOT EDIT.',
        "// Generated by scripts/generate-builtins.php signatures from reflection of PHP {$buildVersions}",
        "// and the PHPDoc types of PHPStan's function map in phpstan-src {$phpstanRef} (MIT License, see NOTICE-PHPStan).",
        '',
        'var generatedBuiltinFunctionSignatures = []builtinFunctionSignature{',
        ...$lines,
        '}',
        '',
    ]);
}

return [
    'name' => 'signatures',
    'usage' => 'php scripts/generate-builtins.php signatures --php=COMMANDS --phpstan=TAG [--extensions-php=COMMANDS] [--pecl-php=COMMANDS] [--output=PATH]',
    'help' => '  signatures    Run the same builds as classes mode and emit the Go table of
                PHP\'s built-in functions with their native signature, from the
                newest PHP that has each, and the PHPDoc types PHPStan\'s
                function map gives them, version by version, where those say
                more than the native types of that version\'s build and allow
                the same kinds of value. --phpstan is the phpstan-src tag
                whose function map is fetched from GitHub, such as 2.3.7.
                Output defaults to
                internal/symbols/builtin_signatures_generated.go.
',
    'run' => static function (array $options): void {
        if ($options['php'] === '' || $options['phpstan'] === null) {
            fwrite(STDERR, "Signatures mode needs PHP's default builds (--php) and a phpstan-src tag (--phpstan).\n");
            exit(1);
        }
        writeOutput(
            generateGoFunctionSignatures(
                runPhpDumps($options['php'], functionDumpCode()),
                runPhpDumps($options['extensions-php'], functionDumpCode()),
                runPhpDumps($options['pecl-php'], functionDumpCode()),
                dirname(__DIR__, 2) . '/internal/stubs/php/extensions',
                $options['phpstan']
            ),
            $options['output'] ?? dirname(__DIR__, 2) . '/internal/symbols/builtin_signatures_generated.go'
        );
    },
];

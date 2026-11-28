<?php
$groups = get_defined_constants(true);
unset($groups['user']);
$out = ['version' => PHP_MAJOR_VERSION . '.' . PHP_MINOR_VERSION, 'release' => PHP_VERSION, 'os' => PHP_OS_FAMILY, 'extensions' => array_map('strtolower', get_loaded_extensions()), 'constants' => []];
foreach ($groups as $extension => $constants) {
    foreach ($constants as $name => $value) {
        $out['constants'][strtolower($extension)][$name] = [gettype($value), is_scalar($value) || $value === null ? var_export($value, true) : null];
    }
}
echo json_encode($out, JSON_INVALID_UTF8_SUBSTITUTE);

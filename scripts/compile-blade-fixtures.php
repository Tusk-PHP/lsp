<?php

declare(strict_types=1);

if ($argc !== 3) {
    fwrite(STDERR, "usage: php scripts/compile-blade-fixtures.php <laravel vendor/autoload.php> <fixture dir>\n");
    exit(2);
}

require $argv[1];

$compiler = new Illuminate\View\Compilers\BladeCompiler(new Illuminate\Filesystem\Filesystem(), sys_get_temp_dir());
$templates = glob(rtrim($argv[2], '/').'/*.blade.php');
if ($templates === [] || $templates === false) {
    fwrite(STDERR, "no templates in {$argv[2]}\n");
    exit(1);
}
foreach ($templates as $template) {
    $compiled = substr($template, 0, -strlen('.blade.php')).'.compiled.php';
    $php = $compiler->compileString(file_get_contents($template));
    $temp = $compiled.'.tmp';
    file_put_contents($temp, $php);
    rename($temp, $compiled);
}

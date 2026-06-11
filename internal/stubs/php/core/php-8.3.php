<?php

// PHP 8.3 delta — only symbols not already in earlier baselines/deltas.
// json_validate() is in internal/stubs/php/extensions/json/php-8.3.php.

/**
 * Attribute that declares a method as intentionally overriding a parent
 * method, causing a compile-time warning if no such parent method exists.
 */
#[\Attribute(\Attribute::TARGET_METHOD)]
final class Override {}

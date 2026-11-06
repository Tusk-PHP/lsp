<?php

// Source: https://github.com/igbinary/igbinary/blob/master/igbinary.php

/** Encode a PHP value in the compact igbinary format; null on serialization failure. */
function igbinary_serialize(mixed $value): ?string {}

/** Restore a PHP value from an igbinary representation; null on decoding failure. */
function igbinary_unserialize(string $str): mixed {}

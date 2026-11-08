<?php

// PHP 8.0 delta: members added in 8.0, merged into the classes declared earlier.

class ValueError extends \Error
{

}

class UnhandledMatchError extends \Error
{

}

interface Stringable
{
    public function __toString(): string;
}

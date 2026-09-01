<?php

namespace App\Bad;

class UndefinedVariable
{
    public function run(): string
    {
        $greeting = 'hello';

        return $greetingText;
    }
}

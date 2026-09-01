<?php

namespace App\Bad;

class UnknownClass
{
    public function run(): object
    {
        return new \App\Support\PostmarkMailer();
    }
}

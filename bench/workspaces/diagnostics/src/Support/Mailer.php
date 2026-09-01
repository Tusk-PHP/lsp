<?php

namespace App\Support;

class Mailer
{
    public function send(string $to, string $subject): bool
    {
        return $to !== '' && $subject !== '';
    }
}

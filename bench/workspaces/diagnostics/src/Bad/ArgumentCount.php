<?php

namespace App\Bad;

use App\Support\Mailer;

class ArgumentCount
{
    public function run(Mailer $mailer): bool
    {
        return $mailer->send('someone@example.test');
    }
}

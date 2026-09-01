<?php

namespace App\Bad;

use App\Support\Mailer;

class UnknownMethod
{
    public function run(Mailer $mailer): bool
    {
        return $mailer->sendMessage('someone@example.test');
    }
}

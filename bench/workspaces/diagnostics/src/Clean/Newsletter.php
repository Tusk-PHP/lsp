<?php

namespace App\Clean;

use App\Support\Mailer;

/**
 * Deliberately unremarkable: every symbol resolves, every argument matches, and
 * nothing is unused. A diagnostic reported here is a false positive.
 */
class Newsletter
{
    public function __construct(private Mailer $mailer)
    {
    }

    public function sendDigest(string $recipient): bool
    {
        $subject = 'Weekly digest';

        return $this->mailer->send($recipient, $subject);
    }
}

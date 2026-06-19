<?php

namespace App\Services;

class Checkout
{
    public function __construct(
        private PaymentGateway $gateway,
    ) {}

    public function process(int $amount): bool
    {
        return $this->gateway->charge($amount);
    }
}

<?php

declare(strict_types=1);

use App\Service\PaymentProcessor;
use Symfony\Component\DependencyInjection\Loader\Configurator\ContainerConfigurator;

return static function (ContainerConfigurator $container): void {
    $container->services()->set('app.package_payment_processor', PaymentProcessor::class);
};

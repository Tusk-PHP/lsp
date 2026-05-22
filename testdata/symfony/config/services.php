<?php

declare(strict_types=1);

use App\Service\NotificationService;
use Symfony\Component\DependencyInjection\Loader\Configurator\ContainerConfigurator;

return static function (ContainerConfigurator $container): void {
    $services = $container->services();

    $services->alias('app.php_notifier', NotificationService::class);
};

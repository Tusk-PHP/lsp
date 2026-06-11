<?php

namespace App\Service;

use Symfony\Component\DependencyInjection\Attribute\Autowire;
use Psr\Log\LoggerInterface;

class AutowiredService
{
    public function __construct(
        #[Autowire(service: 'app.notifier')]
        private NotificationService $notifier,

        #[Autowire('%app.some_param%')]
        private string $someParam,

        #[Autowire(param: 'app.other_param')]
        private string $otherParam,

        #[Autowire(env: 'APP_SECRET')]
        private string $appSecret,

        private LoggerInterface $logger,
    ) {}

    public function doWork(): void
    {
        $this->notifier->notify('Work done');
    }
}

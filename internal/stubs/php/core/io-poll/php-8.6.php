<?php

// PHP 8.6 delta: Io\Poll functions, classes and enums added in 8.6,
// rendered from PHP 8.6.0's reflection. Classes list all their members.

namespace Io\Poll;

enum Backend {
    case Auto;
    case Epoll;
    case EventPorts;
    case Kqueue;
    case Poll;
    case WSAPoll;
    public static function getAvailableBackends(): array {}
    public function isAvailable(): bool {}
    public function supportsEdgeTriggering(): bool {}
}

class BackendUnavailableException extends \Io\Poll\PollException implements \Stringable, \Throwable {
}

final class Context {
    public function __construct(\Io\Poll\Backend $backend = Io\Poll\Backend::Auto) {}
    public function add(\Io\Poll\Handle $handle, array $events, mixed $data = NULL): \Io\Poll\Watcher {}
    public function getBackend(): \Io\Poll\Backend {}
    public function wait(?\Time\Duration $timeout = NULL, ?int $maxEvents = NULL): array {}
}

enum Event {
    case EdgeTriggered;
    case Error;
    case HangUp;
    case OneShot;
    case Read;
    case ReadHangUp;
    case Write;
}

class FailedContextInitializationException extends \Io\Poll\FailedPollOperationException implements \Stringable, \Throwable {
}

class FailedHandleAddException extends \Io\Poll\FailedPollOperationException implements \Stringable, \Throwable {
}

abstract class FailedPollOperationException extends \Io\Poll\PollException implements \Stringable, \Throwable {
    public const ERROR_AGAIN = 10;
    public const ERROR_EXISTS = 4;
    public const ERROR_INTERRUPTED = 7;
    public const ERROR_INVALID = 3;
    public const ERROR_NOMEM = 2;
    public const ERROR_NONE = 0;
    public const ERROR_NOSUPPORT = 11;
    public const ERROR_NOTFOUND = 5;
    public const ERROR_PERMISSION = 8;
    public const ERROR_SYSTEM = 1;
    public const ERROR_TIMEOUT = 6;
    public const ERROR_TOOBIG = 9;
}

class FailedPollWaitException extends \Io\Poll\FailedPollOperationException implements \Stringable, \Throwable {
}

class FailedWatcherModificationException extends \Io\Poll\FailedPollOperationException implements \Stringable, \Throwable {
}

interface Handle {
}

class HandleAlreadyWatchedException extends \Io\Poll\PollException implements \Stringable, \Throwable {
}

class InactiveWatcherException extends \Io\Poll\PollException implements \Stringable, \Throwable {
}

class InvalidHandleException extends \Io\Poll\PollException implements \Stringable, \Throwable {
}

class PollException extends \Io\IoException implements \Stringable, \Throwable {
}

final class Watcher {
    private final function __construct() {}
    public function getData(): mixed {}
    public function getHandle(): \Io\Poll\Handle {}
    public function getTriggeredEvents(): array {}
    public function getWatchedEvents(): array {}
    public function hasTriggered(\Io\Poll\Event $event): bool {}
    public function isActive(): bool {}
    public function modify(array $events, mixed $data = NULL): void {}
    public function modifyData(mixed $data): void {}
    public function modifyEvents(array $events): void {}
    public function remove(): void {}
}

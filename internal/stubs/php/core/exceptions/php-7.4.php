<?php

// PHP 7.4 baseline. Signatures from PHP's own reflection; members added in later
// versions are in the matching php-X.Y.php delta.

interface Throwable
{
    public function getCode();

    public function getFile(): string;

    public function getLine(): int;

    public function getMessage(): string;

    public function getPrevious(): ?\Throwable;

    public function getTrace(): array;

    public function getTraceAsString(): string;
}

class Exception implements \Throwable
{
    protected $code;

    protected string $file;

    protected int $line;

    protected $message;

    private ?\Throwable $previous;

    private string $string;

    private array $trace;

    private function __clone(): void {}

    public function __construct(string $message = '', int $code = 0, ?\Throwable $previous = null) {}

    public function __toString(): string {}

    public function __wakeup(): void {}

    public final function getCode() {}

    public final function getFile(): string {}

    public final function getLine(): int {}

    public final function getMessage(): string {}

    public final function getPrevious(): ?\Throwable {}

    public final function getTrace(): array {}

    public final function getTraceAsString(): string {}
}

class ErrorException extends \Exception
{
    protected int $severity;

    public function __construct(string $message = '', int $code = 0, int $severity = E_ERROR, ?string $filename = null, ?int $line = null, ?\Throwable $previous = null) {}

    public final function getSeverity(): int {}
}

class Error implements \Throwable
{
    protected $code;

    protected string $file;

    protected int $line;

    protected $message;

    private ?\Throwable $previous;

    private string $string;

    private array $trace;

    private function __clone(): void {}

    public function __construct(string $message = '', int $code = 0, ?\Throwable $previous = null) {}

    public function __toString(): string {}

    public function __wakeup(): void {}

    public final function getCode() {}

    public final function getFile(): string {}

    public final function getLine(): int {}

    public final function getMessage(): string {}

    public final function getPrevious(): ?\Throwable {}

    public final function getTrace(): array {}

    public final function getTraceAsString(): string {}
}

class TypeError extends \Error
{

}

class ArithmeticError extends \Error
{

}

class DivisionByZeroError extends \ArithmeticError
{

}

class ArgumentCountError extends \TypeError
{

}

class CompileError extends \Error
{

}

class ParseError extends \CompileError
{

}

class LogicException extends \Exception
{

}

class BadFunctionCallException extends \LogicException
{

}

class BadMethodCallException extends \BadFunctionCallException
{

}

class DomainException extends \LogicException
{

}

class InvalidArgumentException extends \LogicException
{

}

class LengthException extends \LogicException
{

}

class OutOfRangeException extends \LogicException
{

}

class RuntimeException extends \Exception
{

}

class OutOfBoundsException extends \RuntimeException
{

}

class OverflowException extends \RuntimeException
{

}

class RangeException extends \RuntimeException
{

}

class UnderflowException extends \RuntimeException
{

}

class UnexpectedValueException extends \RuntimeException
{

}

class JsonException extends \Exception
{

}

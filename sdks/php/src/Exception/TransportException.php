<?php

declare(strict_types=1);

namespace ClickFix\Exception;

/**
 * The HTTP client could not complete the request (DNS, connection, timeout) after the retry budget.
 */
final class TransportException extends \RuntimeException implements ClickFixException
{
}

<?php

// PHP 8.6 delta: Uri\Rfc3986 functions, classes and enums added in 8.6,
// rendered from PHP 8.6.0's reflection. Classes list all their members.

namespace Uri\Rfc3986;

final class UriBuilder {
    private ?string $fragment;
    private ?string $host;
    private string $path;
    private ?int $port;
    private ?string $query;
    private ?string $scheme;
    private ?string $userinfo;
    public function build(?\Uri\Rfc3986\Uri $baseUrl = NULL): \Uri\Rfc3986\Uri {}
    public function reset(): static {}
    public function setFragment(?string $fragment): static {}
    public function setHost(?string $host): static {}
    public function setPath(string $path): static {}
    public function setPort(?int $port): static {}
    public function setQuery(?string $query): static {}
    public function setScheme(?string $scheme): static {}
    public function setUserInfo(?string $userInfo): static {}
}

enum UriHostType {
    case IpV4;
    case IpV6;
    case IpVFuture;
    case RegisteredName;
}

enum UriType {
    case AbsolutePathReference;
    case NetworkPathReference;
    case RelativePathReference;
    case Uri;
}

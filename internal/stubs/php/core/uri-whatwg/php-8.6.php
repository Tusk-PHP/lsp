<?php

// PHP 8.6 delta: Uri\WhatWg functions, classes and enums added in 8.6,
// rendered from PHP 8.6.0's reflection. Classes list all their members.

namespace Uri\WhatWg;

final class UrlBuilder {
    private ?string $fragment;
    private ?string $host;
    private ?string $password;
    private string $path;
    private ?int $port;
    private ?string $query;
    private string $scheme;
    private ?string $username;
    public function build(?\Uri\WhatWg\Url $baseUrl = NULL, &$softErrors = NULL): \Uri\WhatWg\Url {}
    public function reset(): static {}
    public function setFragment(?string $fragment): static {}
    public function setHost(?string $host): static {}
    public function setPassword(?string $password): static {}
    public function setPath(string $path): static {}
    public function setPort(?int $port): static {}
    public function setQuery(?string $query): static {}
    public function setScheme(string $scheme): static {}
    public function setUsername(?string $username): static {}
}

enum UrlHostType {
    case Domain;
    case Empty;
    case IpV4;
    case IpV6;
    case Opaque;
}

enum UrlPercentEncodingMode {
    case FormQuery;
    case Fragment;
    case OpaqueHost;
    case OpaquePath;
    case Password;
    case Path;
    case PathSegment;
    case Query;
    case SpecialQuery;
    case Username;
}

function url_percent_encode(string $input, \Uri\WhatWg\UrlPercentEncodingMode $mode): string {}

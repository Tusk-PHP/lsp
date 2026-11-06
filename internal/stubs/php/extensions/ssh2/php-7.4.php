<?php

// Source: php/pecl-networking-ssh2: ssh2.c, ssh2_sftp.c, ssh2_fopen_wrappers.c.
// Resources use PHPDoc because resource is not a native PHP type.

/**
 * Connect to an SSH server.
 * @return resource|false
 */
function ssh2_connect(string $host, int $port = 22, ?array $methods = null, ?array $callbacks = null) {}
/**
 * Close an SSH session.
 * @param resource $session
 */
function ssh2_disconnect($session): bool {}
/**
 * Set the blocking session timeout.
 * @param resource $session
 */
function ssh2_set_timeout($session, int $seconds, int $microseconds = 0): void {}
/**
 * Configure keepalive messages.
 * @param resource $session
 */
function ssh2_keepalive_config($session, bool $want_reply, int $interval): void {}
/**
 * Send a keepalive and return seconds until the next one.
 * @param resource $session
 */
function ssh2_keepalive_send($session): int|false {}
/**
 * Return negotiated connection algorithms.
 * @param resource $session
 */
function ssh2_methods_negotiated($session): array {}
/**
 * Return a server host-key fingerprint.
 * @param resource $session
 */
function ssh2_fingerprint($session, int $flags = SSH2_FINGERPRINT_MD5 | SSH2_FINGERPRINT_HEX): string|false {}
/**
 * Return the host key and its type.
 * @param resource $session
 */
function ssh2_hostkey($session): array|false {}
/**
 * Try unauthenticated access or return supported authentication methods.
 * @param resource $session
 */
function ssh2_auth_none($session, string $username): array|bool {}
/**
 * Authenticate with a password.
 * @param resource $session
 */
function ssh2_auth_password($session, string $username, string $password): bool {}
/**
 * Authenticate with public/private key files.
 * @param resource $session
 */
function ssh2_auth_pubkey_file($session, string $username, string $publickeyfile, string $privatekeyfile, string $passphrase = ''): bool {}
/**
 * Authenticate with public/private key strings.
 * @param resource $session
 */
function ssh2_auth_pubkey($session, string $username, string $publickey, string $privatekey, string $passphrase = ''): bool {}
/**
 * Authenticate with a trusted host key.
 * @param resource $session
 */
function ssh2_auth_hostbased_file($session, string $username, string $hostname, string $publickeyfile, string $privatekeyfile, string $passphrase = '', ?string $local_username = null): bool {}
/**
 * Authenticate through the SSH agent.
 * @param resource $session
 */
function ssh2_auth_agent($session, string $username): bool {}
/**
 * Listen for a forwarded connection.
 * @param resource $session
 * @return resource|false
 */
function ssh2_forward_listen($session, int $port, ?string $host = null, int $max_connections = 16) {}
/**
 * Accept a forwarded connection.
 * @param resource $listener
 * @return resource|false
 */
function ssh2_forward_accept($listener) {}
/**
 * Open an interactive shell.
 * @param resource $session
 * @return resource|false
 */
function ssh2_shell($session, string $term_type = SSH2_DEFAULT_TERMINAL, ?array $env = null, int $width = SSH2_DEFAULT_TERM_WIDTH, int $height = SSH2_DEFAULT_TERM_HEIGHT, int $width_height_type = SSH2_DEFAULT_TERM_UNIT) {}
/**
 * Resize an interactive terminal.
 * @param resource $channel
 */
function ssh2_shell_resize($channel, int $width, int $height, int $width_px = 0, int $height_px = 0): bool {}
/**
 * Execute a remote command.
 * @param resource $session
 * @return resource|false
 */
function ssh2_exec($session, string $command, ?string $pty = null, ?array $env = null, int $width = SSH2_DEFAULT_TERM_WIDTH, int $height = SSH2_DEFAULT_TERM_HEIGHT, int $width_height_type = SSH2_DEFAULT_TERM_UNIT) {}
/**
 * Tunnel a TCP connection through SSH.
 * @param resource $session
 * @return resource|false
 */
function ssh2_tunnel($session, string $host, int $port) {}
/**
 * Receive a file through SCP.
 * @param resource $session
 */
function ssh2_scp_recv($session, string $remote_file, string $local_file): bool {}
/**
 * Send a file through SCP.
 * @param resource $session
 */
function ssh2_scp_send($session, string $local_file, string $remote_file, int $create_mode = 0644): bool {}
/**
 * Select a channel's standard output or error stream.
 * @param resource $channel
 * @return resource|false
 */
function ssh2_fetch_stream($channel, int $streamid) {}
/** Poll SSH channels, listeners and sessions. */
function ssh2_poll(array &$polldes, int $timeout = 30): int {}
/**
 * Send a signal to a channel.
 * @param resource $channel
 */
function ssh2_send_signal($channel, string $signal): bool {}
/**
 * Send end-of-file to a channel.
 * @param resource $channel
 */
function ssh2_send_eof($channel): bool {}
/**
 * Initialize an SFTP subsystem.
 * @param resource $session
 * @return resource|false
 */
function ssh2_sftp($session) {}
/**
 * Rename a remote file.
 * @param resource $sftp
 */
function ssh2_sftp_rename($sftp, string $from, string $to): bool {}
/**
 * Delete a remote file.
 * @param resource $sftp
 */
function ssh2_sftp_unlink($sftp, string $filename): bool {}
/**
 * Create a remote directory.
 * @param resource $sftp
 */
function ssh2_sftp_mkdir($sftp, string $dirname, int $mode = 0777, bool $recursive = false): bool {}
/**
 * Remove a remote directory.
 * @param resource $sftp
 */
function ssh2_sftp_rmdir($sftp, string $dirname): bool {}
/**
 * Change remote file permissions.
 * @param resource $sftp
 */
function ssh2_sftp_chmod($sftp, string $filename, int $mode): bool {}
/**
 * Stat a remote file following symbolic links.
 * @param resource $sftp
 */
function ssh2_sftp_stat($sftp, string $path): array|false {}
/**
 * Stat a remote file without following symbolic links.
 * @param resource $sftp
 */
function ssh2_sftp_lstat($sftp, string $path): array|false {}
/**
 * Create a remote symbolic link.
 * @param resource $sftp
 */
function ssh2_sftp_symlink($sftp, string $target, string $link): bool {}
/**
 * Read a remote symbolic link.
 * @param resource $sftp
 */
function ssh2_sftp_readlink($sftp, string $link): string|false {}
/**
 * Resolve a remote path.
 * @param resource $sftp
 */
function ssh2_sftp_realpath($sftp, string $filename): string|false {}
/**
 * Initialize the public-key management subsystem.
 * @param resource $session
 * @return resource|false
 */
function ssh2_publickey_init($session) {}
/**
 * Add an authorized public key.
 * @param resource $pkey
 */
function ssh2_publickey_add($pkey, string $algoname, string $blob, bool $overwrite = false, ?array $attributes = null): bool {}
/**
 * Remove an authorized public key.
 * @param resource $pkey
 */
function ssh2_publickey_remove($pkey, string $algoname, string $blob): bool {}
/**
 * List authorized public keys.
 * @param resource $pkey
 */
function ssh2_publickey_list($pkey): array|false {}

// Constants registered by SSH2 (including optional libssh2 features).
const SSH2_FINGERPRINT_MD5 = 0x0000;
const SSH2_FINGERPRINT_SHA1 = 0x0001;
const SSH2_FINGERPRINT_HEX = 0x0000;
const SSH2_FINGERPRINT_RAW = 0x0002;
const SSH2_FINGERPRINT_SHA256 = 0x0004;
const SSH2_HOSTKEY_TYPE_UNKNOWN = 0;
const SSH2_HOSTKEY_TYPE_RSA = 1;
const SSH2_HOSTKEY_TYPE_DSS = 2;
const SSH2_HOSTKEY_TYPE_ECDSA_256 = 3;
const SSH2_HOSTKEY_TYPE_ECDSA_384 = 4;
const SSH2_HOSTKEY_TYPE_ECDSA_521 = 5;
const SSH2_HOSTKEY_TYPE_ED25519 = 6;
const SSH2_TERM_UNIT_CHARS = 0x0000;
const SSH2_TERM_UNIT_PIXELS = 0x0001;
const SSH2_DEFAULT_TERMINAL = "vanilla";
const SSH2_DEFAULT_TERM_WIDTH = 80;
const SSH2_DEFAULT_TERM_HEIGHT = 25;
const SSH2_DEFAULT_TERM_UNIT = 0x0000;
const SSH2_STREAM_STDIO = 0;
const SSH2_STREAM_STDERR = 1;
const SSH2_POLLIN = 0x0001;
const SSH2_POLLEXT = 0x0002;
const SSH2_POLLOUT = 0x0004;
const SSH2_POLLERR = 0x0008;
const SSH2_POLLHUP = 0x0010;
const SSH2_POLLNVAL = 0x0020;
const SSH2_POLL_SESSION_CLOSED = 0x0010;
const SSH2_POLL_CHANNEL_CLOSED = 0x0080;
const SSH2_POLL_LISTENER_CLOSED = 0x0080;

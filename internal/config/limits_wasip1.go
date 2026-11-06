//go:build wasip1

package config

// defaultMaxIndexFiles keeps the WebAssembly build (vscode.dev and other
// browser hosts) inside its memory budget of about 448 MB. Without a cap,
// a Magento 2-sized workspace needs about 614 MB.
const defaultMaxIndexFiles = 10000

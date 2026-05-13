//go:build windows

package main

import _ "embed"

//go:embed Everything64.dll
var embeddedDLL []byte

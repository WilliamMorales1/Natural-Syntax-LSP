// Package scripts embeds the language definitions written in Lua.
package scripts

import "embed"

// FS holds one *.lua file per scripted language.
//
//go:embed *.lua
var FS embed.FS

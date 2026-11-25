package scope

import "slices"

var autoGlobals = []string{"$GLOBALS", "$_COOKIE", "$_ENV", "$_FILES", "$_GET", "$_POST", "$_REQUEST", "$_SERVER", "$_SESSION"}

func AutoGlobals() []string {
	return slices.Clone(autoGlobals)
}

func IsAutoGlobal(name string) bool {
	return slices.Contains(autoGlobals, name)
}

package main

import "strings"

// flattenDirPath accepts a file directory and flattens it, replacing `/` with `$`
// Obviously this is bad, but let's go with it.
func flattenDirPath(path string) string {
	return strings.ReplaceAll(path, "/", "$")
}
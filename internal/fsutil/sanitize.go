package fsutil

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxFilenameBytes = 255

var windowsReserved = regexp.MustCompile(`(?i)^(CON|PRN|AUX|NUL|COM[0-9]|LPT[0-9])(\.|$)`)

func SanitizeName(name string) string {
	if name == "" {
		return "_"
	}

	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "\x00", "")

	for _, c := range []string{"<", ">", ":", "\"", "|", "?", "*"} {
		name = strings.ReplaceAll(name, c, "_")
	}

	name = strings.TrimLeft(name, ". ")
	name = strings.TrimRight(name, ". ")

	if windowsReserved.MatchString(name) {
		name = "_" + name
	}

	name = truncateToBytes(name, maxFilenameBytes)

	if name == "" {
		return "_"
	}

	return name
}

func SanitizePath(name string) string {
	return SanitizeName(name)
}

func truncateToBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	ext := filepath.Ext(s)
	if len(ext) > 0 && len(ext) < max-10 {
		base := s[:len(s)-len(ext)]
		base = truncateUTF8(base, max-len(ext))
		return base + ext
	}
	return truncateUTF8(s, max)
}

func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}

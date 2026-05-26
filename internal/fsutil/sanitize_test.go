package fsutil

import (
	"strings"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", "_"},
		{"simple", "hello.txt", "hello.txt"},
		{"forward slash", "path/to/file.txt", "path_to_file.txt"},
		{"backslash", "path\\to\\file.txt", "path_to_file.txt"},
		{"null byte", "file\x00name.txt", "filename.txt"},
		{"angle brackets", "file<name>.txt", "file_name_.txt"},
		{"colon", "file:name.txt", "file_name.txt"},
		{"quotes", "file\"name\".txt", "file_name_.txt"},
		{"pipe", "file|name.txt", "file_name.txt"},
		{"question mark", "file?name.txt", "file_name.txt"},
		{"asterisk", "file*name.txt", "file_name.txt"},
		{"leading dots", "...hidden", "hidden"},
		{"trailing dots", "file...", "file"},
		{"leading spaces", "  file.txt", "file.txt"},
		{"trailing spaces", "file.txt  ", "file.txt"},
		{"only dots", "...", "_"},
		{"only spaces", "   ", "_"},
		{"CON reserved", "CON", "_CON"},
		{"CON.txt reserved", "CON.txt", "_CON.txt"},
		{"con lowercase reserved", "con", "_con"},
		{"COM1 reserved", "COM1", "_COM1"},
		{"LPT3 reserved", "LPT3", "_LPT3"},
		{"NUL reserved", "NUL", "_NUL"},
		{"PRN reserved", "PRN", "_PRN"},
		{"AUX reserved", "AUX", "_AUX"},
		{"CONX not reserved", "CONX", "CONX"},
		{"unicode preserved", "日本語ファイル.txt", "日本語ファイル.txt"},
		{"mixed bad chars", "a/b\\c<d>e:f\"g|h?i*j", "a_b_c_d_e_f_g_h_i_j"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeName(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeName_TruncatesLongNames(t *testing.T) {
	long := strings.Repeat("a", 300)
	got := SanitizeName(long)
	if len(got) > maxFilenameBytes {
		t.Errorf("len=%d, want <= %d", len(got), maxFilenameBytes)
	}
}

func TestSanitizeName_TruncatePreservesExtension(t *testing.T) {
	long := strings.Repeat("a", 300) + ".txt"
	got := SanitizeName(long)
	if len(got) > maxFilenameBytes {
		t.Errorf("len=%d, want <= %d", len(got), maxFilenameBytes)
	}
	if !strings.HasSuffix(got, ".txt") {
		t.Errorf("expected .txt suffix, got %q", got)
	}
}

func TestSanitizeName_TruncateUTF8Safe(t *testing.T) {
	// Each Japanese character is 3 bytes in UTF-8
	long := strings.Repeat("あ", 100) + ".txt" // 300 + 4 = 304 bytes
	got := SanitizeName(long)
	if len(got) > maxFilenameBytes {
		t.Errorf("len=%d, want <= %d", len(got), maxFilenameBytes)
	}
	if !strings.HasSuffix(got, ".txt") {
		t.Errorf("expected .txt suffix, got %q", got)
	}
	// Verify valid UTF-8 (no truncation mid-rune)
	for i := 0; i < len(got); {
		r, size := []rune(got[i:])[0], len(string([]rune(got[i:])[0]))
		if r == 0xFFFD && got[i] != 0xEF {
			t.Errorf("invalid UTF-8 at byte %d", i)
		}
		i += size
	}
}

func TestSanitizePath(t *testing.T) {
	got := SanitizePath("My Album / Special <Edition>")
	if strings.ContainsAny(got, "/\\<>") {
		t.Errorf("SanitizePath should remove path separators and angle brackets, got %q", got)
	}
}

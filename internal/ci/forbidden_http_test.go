package ci

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestForbiddenDirectHTTP scans repository Go files for direct http client usage
// patterns (http.Post or client.Do) outside of internal/billing package.
func TestForbiddenDirectHTTP(t *testing.T) {
	root := "./"
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`http\.Post\s*\(`),
		regexp.MustCompile(`client\.Do\s*\(`),
	}

	var offenders []string

	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// skip directories we don't want to scan
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// allow anything under internal/billing
		if strings.Contains(path, filepath.Join("internal", "billing")) {
			return nil
		}
		// read file
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		s := string(data)
		for _, re := range patterns {
			if re.MatchString(s) {
				offenders = append(offenders, path+": pattern -> "+re.String())
			}
		}
		return nil
	}

	if err := filepath.WalkDir(root, walkFn); err != nil {
		t.Fatalf("walk failed: %v", err)
	}

	if len(offenders) > 0 {
		t.Fatalf("forbidden direct HTTP usage found in files:\n%s", strings.Join(offenders, "\n"))
	}
}

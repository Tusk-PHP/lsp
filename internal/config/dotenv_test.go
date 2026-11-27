package config

import (
	"slices"
	"testing"
)

func TestParseDotenvReadsKeysValuesAndWhereTheyAre(t *testing.T) {
	content := "# App\n" +
		"APP_NAME=Laravel\n" +
		"\n" +
		"export APP_ENV=local\n" +
		"  APP_DEBUG = true # inline comment\n" +
		"APP_URL=http://localhost#not-a-comment\n" +
		"EMPTY=\n" +
		"SINGLE='it\\'s literal \\n'\n" +
		"DOUBLE=\"tab\\there \\\"quoted\\\"\"\r\n" +
		"not an assignment\n" +
		"export\n" +
		"BAD-KEY=x\n" +
		"LAST=1"
	want := []DotenvEntry{
		{Key: "APP_NAME", Value: "Laravel", Line: 1, Column: 0},
		{Key: "APP_ENV", Value: "local", Line: 3, Column: 7},
		{Key: "APP_DEBUG", Value: "true", Line: 4, Column: 2},
		{Key: "APP_URL", Value: "http://localhost#not-a-comment", Line: 5, Column: 0},
		{Key: "EMPTY", Value: "", Line: 6, Column: 0},
		{Key: "SINGLE", Value: `it's literal \n`, Line: 7, Column: 0},
		{Key: "DOUBLE", Value: "tab\there \"quoted\"", Line: 8, Column: 0},
		{Key: "LAST", Value: "1", Line: 12, Column: 0},
	}
	if got := ParseDotenv(content); !slices.Equal(got, want) {
		t.Errorf("ParseDotenv =\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseDotenvSkipsTheLinesOfAQuotedValue(t *testing.T) {
	content := "PRIVATE_KEY=\"-----BEGIN KEY-----\n" +
		"NOT_A_KEY=inside the value\n" +
		"# nor a comment\n" +
		"-----END KEY-----\"\n" +
		"SINGLE='one\n" +
		"ALSO_INSIDE=1\n" +
		"'\n" +
		"AFTER=yes\n"
	var keys []string
	for _, entry := range ParseDotenv(content) {
		keys = append(keys, entry.Key)
	}
	if want := []string{"PRIVATE_KEY", "SINGLE", "AFTER"}; !slices.Equal(keys, want) {
		t.Errorf("keys = %v, want %v", keys, want)
	}
}

func TestParseDotenvUnterminatedQuoteTakesTheRestOfTheFile(t *testing.T) {
	entries := ParseDotenv("OPEN=\"never closed\nHIDDEN=1\n")
	if len(entries) != 1 || entries[0].Key != "OPEN" {
		t.Errorf("an unterminated value hides no keys after it, as phpdotenv fails on it: %+v", entries)
	}
}

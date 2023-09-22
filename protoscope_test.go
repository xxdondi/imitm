package main

import (
	"os"
	"strings"
	"testing"
)

func TestFindMismatchedBrackets(t *testing.T) {
	testStr1 := "foo { bar { baz } something else"
	testStr2 := "foo { bar } } something else"
	testStr3 := "foo { bar } blah blah"

	result := findMismatchedBracket(testStr1)
	if result != 32 {
		t.Errorf("findMismatchedBracket(%v) should return 32, got %d", testStr1, result)
	}
	result = findMismatchedBracket(testStr2)
	if result != 12 {
		t.Errorf("findMismatchedBracket(%v) should return 12, got %d", testStr2, result)
	}
	result = findMismatchedBracket(testStr3)
	if result != -1 {
		t.Errorf("findMismatchedBracket(%v) should return -1, got %d", testStr3, result)
	}
}

func TestFindMismatchedBracketsRight(t *testing.T) {
	testStr1 := "foo { bar { baz } something else"
	testStr2 := "foo { bar } } something else"
	testStr3 := "foo { bar } blah blah"

	result := findMismatchedBracketRight(testStr1)
	if result != 4 {
		t.Errorf("findMismatchedBracketRight(%v) should return 4, got %d", testStr1, result)
	}
	result = findMismatchedBracketRight(testStr2)
	if result != -1 {
		t.Errorf("findMismatchedBracketRight(%v) should return -1, got %d", testStr2, result)
	}
	result = findMismatchedBracketRight(testStr3)
	if result != -1 {
		t.Errorf("findMismatchedBracketRight(%v) should return -1, got %d", testStr3, result)
	}
}

func TestFindParentKeyIdx(t *testing.T) {
	testStr1 := `
		6 {
			1: 4
			2 {
				1: "context"
				2: "yt_ios_small_form_factor_w2w"
			}
			2 {
				1: "logged_in"
				2: "1"
			}
		}
	`

	result := findParentKeyIdx(testStr1, 8)
	if result != 3 {
		t.Errorf("findParentKeyIdx(5) should return 3, got %d", result)
	}
	result = findParentKeyIdx(testStr1, 29)
	if result != 18 {
		t.Errorf("findParentKeyIdx(5) should return 18, got %d", result)
	}
	result = findParentKeyIdx(testStr1, 0)
	if result != -1 {
		t.Errorf("findParentKeyIdx(0) should return -1, got %d", result)
	}
}

func TestCorruptKeyAt(t *testing.T) {
	testStr1 := `
		6 {
			1: 4
			2 {
				1: "context"
				2: "yt_ios_small_form_factor_w2w"
			}
			2 {
				1: "logged_in"
				2: "1"
			}
		}
	`
	result := corruptKeyAt(testStr1, 3)
	if result[3:6] != "999" {
		t.Errorf("corruptKeyAt(3)[3:6] should be '999', got %s", string(result[3:6]))
	}
}

func TestCorruptNthParentKey(t *testing.T) {
	testStr1 := `
		6 {
			1: 4
			2 {
				1: "context"
				2: "yt_ios_small_form_factor_w2w"
			}
			2 {
				1: "logged_in"
				2: "1"
			}
		}
	`
	result := CorruptNthParentKeyFn(1)(testStr1, 9, 10)
	if result[3:6] != "999" {
		t.Errorf("CorruptNthParentKeyFn(1)(9, 10)[3:6] should be '999', got %s", string(result[3:6]))
	}
}

func TestCorrupt(t *testing.T) {
	doc := NewProtoscopeDoc(`foo: { 123: { baz: "asd" } }`)
	doc.Corrupt(NewProtoCorruptKeyRule("123", FieldValueContains("asd")))
	if doc.String() != `foo: { 999: { baz: "asd" } }` {
		t.Errorf("Expected output mismatch, got %v", doc.text)
	}
}

func TestCorrupt1(t *testing.T) {
	file, err := os.ReadFile("test_data/youtube-videoads.pscope.txt")
	if err != nil {
		t.Errorf("Failed to read test data: %v", err)
		return
	}
	doc := NewProtoscopeDoc(string(file))
	doc.Corrupt(NewProtoCorruptKeyRule("412326366", FieldValueContains("https://www.googleadservices.com/pagead/")))
	modified := doc.String()

	lines := strings.Split(modified, "\n")
	// Line where the text should be modified
	if !strings.Contains(lines[3324], "412326297") {
		t.Errorf("Expected 412326297, got %v", lines[3324])
	}
	// Line where the same text should NOT be modified
	if !strings.Contains(lines[4460], "412326366") {
		t.Errorf("Expected 412326366, got %v", lines[4461])
	}
	// And another line where text should NOT be modified
	if !strings.Contains(lines[4463], "412326366") {
		t.Errorf("Expected 412326366, got %v", lines[4463])
	}
}

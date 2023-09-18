package main

import (
	"os"
	"testing"
)

func TestFindMismatchedBrackets(t *testing.T) {
	testStr1 := "foo { bar { baz } something else"
	testStr2 := "foo { bar } } something else"
	testStr3 := "foo { bar } blah blah"

	result := findMismatchedBracket(testStr1)
	if result != 17 {
		t.Errorf("findMismatchedBracket(%v) should return 17, got %d", testStr1, result)
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

func TestCorrupt(t *testing.T) {
	doc := NewProtoscopeDoc(`foo: { 123: { baz: "asd" } }`)
	doc.Corrupt(NewProtoCorruptKeyRule("123", FieldValueContains("asd")))
	if doc.text != `foo: { 122: { baz: "asd" } }` {
		t.Errorf("Corrupt should not have changed the doc text")
	} else {
		os.WriteFile("tst.txt", []byte(doc.text), 0664)
		t.Logf("Corrupted")
	}
}

func TestCorrupt1(t *testing.T) {
	file, err := os.ReadFile("test_data/youtube-videoads.scope.pb.txt")
	if err != nil {
		t.Errorf("Failed to read test data: %v", err)
		return
	}
	doc := NewProtoscopeDoc(string(file))
	doc.Corrupt(NewProtoCorruptKeyRule("412326366", FieldValueContains("https://www.googleadservices.com/pagead/")))
	os.WriteFile("tst.txt", []byte(doc.text), 0664)
}

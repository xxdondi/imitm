package main

import (
	"strconv"
	"strings"

	"github.com/gobwas/glob/util/runes"
)

type ProtoscopeDoc struct {
	text string
}

type ProtoCorruptKeyRule struct {
	key       string
	predicate func(string) bool
}

const CORRUPTION_CONSTANT = 69

func NewProtoCorruptKeyRule(key string, predicate func(string) bool) *ProtoCorruptKeyRule {
	return &ProtoCorruptKeyRule{key: key, predicate: predicate}
}

func FieldValueContains(s string) func(string) bool {
	return func(text string) bool {
		return strings.Contains(text, s)
	}
}

func allIndicesOf(text string, substr string) []int {
	indices := []int{}
	for i := 0; i < len(text); i++ {
		if strings.HasPrefix(text[i:], substr) {
			indices = append(indices, i)
		}
	}
	return indices
}

const OPEN_BRACKET = '{'
const CLOSE_BRACKET = '}'

func findMismatchedBracket(s string) int {
	specialChars := []rune{OPEN_BRACKET, CLOSE_BRACKET}
	// set up stack and map
	st := []rune{}

	// loop over string
	for idx, r := range s {
		if runes.Contains(specialChars, []rune{r}) {
			// if the current character is in the open map,
			// put its closer into the stack and continue
			if r == OPEN_BRACKET {
				st = append(st, CLOSE_BRACKET)
				continue
			} else if r == CLOSE_BRACKET {
				// otherwise, we're dealing with a closer
				// check to make sure the stack isn't empty
				// and whether the top of the stack matches
				// the current character
				l := len(st) - 1
				if l < 0 || r != st[l] {
					return idx
				}
				// take the last element off the stack
				st = st[:l]
			}
		}
	}
	if len(st) == 0 {
		return -1
	} else {
		return len(s)
	}
}

func findFieldEnd(text string) int {
	nextLineIdx := strings.Index(text, "\n")
	return findMismatchedBracket(text[nextLineIdx+1:])
}

func NewProtoscopeDoc(text string) *ProtoscopeDoc {
	return &ProtoscopeDoc{text: text}
}

func corruptKey(key string) string {
	keyNum, err := strconv.Atoi(key)
	if err != nil {
		panic(err)
	}
	keyNum = keyNum - CORRUPTION_CONSTANT
	return strconv.Itoa(keyNum)
}

func (p *ProtoscopeDoc) ToString() string {
	return p.text
}

func (p *ProtoscopeDoc) Corrupt(rule *ProtoCorruptKeyRule) string {
	indices := allIndicesOf(p.text, rule.key)
	if len(indices) == 0 {
		return p.text
	}
	for _, index := range indices {
		endIdx := findFieldEnd(p.text[index:])
		corruptedKey := corruptKey(rule.key)
		if rule.predicate(p.text[index : index+endIdx]) {
			p.text = p.text[:index] + corruptedKey + p.text[index+len(rule.key):]
		}
	}
	return ""
}

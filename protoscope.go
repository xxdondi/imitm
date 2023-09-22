package main

import (
	"log"
	"strconv"
	"strings"

	"github.com/gobwas/glob/util/runes"
	"github.com/protocolbuffers/protoscope"
)

type CorruptFn func(string, int, int) string

type ProtoscopeDoc struct {
	text string
}

type ProtoCorruptKeyRule struct {
	key       string
	predicate func(string) bool
	corrupt   CorruptFn
}

const CORRUPTION_CONSTANT = 69

func NewProtoCorruptKeyRule(key string, predicate func(string) bool) *ProtoCorruptKeyRule {
	return &ProtoCorruptKeyRule{key: key, predicate: predicate}
}

func (p *ProtoCorruptKeyRule) WithCorruptFn(corrupt CorruptFn) *ProtoCorruptKeyRule {
	p.corrupt = corrupt
	return p
}

func CorruptNthParentKeyFn(n int) CorruptFn {
	if n < 1 {
		panic("n must be greater than 0")
	}
	return func(der string, startIdx int, endIdx int) string {
		currentIdx := startIdx
		for i := 0; i < n; i++ {
			parentIdx := findParentKeyIdx(der, currentIdx)
			if parentIdx == -1 {
				log.Printf("[WARN] Could not find parent key for %v", der[startIdx:endIdx])
				return der
			}
			currentIdx = parentIdx
		}
		if currentIdx != startIdx {
			return corruptKeyAt(der, currentIdx)
		}
		return der
	}
}

func FieldValueContains(s string) func(string) bool {
	return func(text string) bool {
		return strings.Contains(text, s)
	}
}

func DeserializeProto(body []byte) string {
	return protoscope.Write(body, protoscope.WriterOptions{
		AllFieldsAreMessages:   false,
		ExplicitLengthPrefixes: false,
		NoGroups:               true,
		PrintFieldNames:        false,
	})
}

func SerializeProto(derString string) []byte {
	scanner := protoscope.NewScanner(derString)
	outBytes, err := scanner.Exec()
	if err != nil {
		log.Printf("Error scanning - %v", err)
		return nil
	}
	return outBytes
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

func findMismatchedBracketRight(s string) int {
	specialChars := []rune{OPEN_BRACKET, CLOSE_BRACKET}
	// set up stack and map
	st := []rune{}

	// loop backwards over a string
	for idx := len(s) - 1; idx >= 0; idx-- {
		r := rune(s[idx])
		if runes.Contains(specialChars, []rune{r}) {
			// if the current character is in the open map,
			// put its closer into the stack and continue
			if r == CLOSE_BRACKET {
				st = append(st, OPEN_BRACKET)
				continue
			} else if r == OPEN_BRACKET {
				// otherwise, we're dealing with a closer
				// check to make sure the stack isn't empty
				// and whether the top of the stack matches
				// the current character
				l := len(st) - 1
				// Stack is mepty
				if l < 0 {
					return idx
				}
				// Stack is not empty, but the top of the stack
				// doesn't match the expected bracket
				if r != st[l] {
					return idx
				}
				// Take the last element off of the stack
				st = st[:l]
			}
		}
	}
	return -1
}

func findMismatchedBracket(s string) int {
	specialChars := []rune{OPEN_BRACKET, CLOSE_BRACKET}
	// set up stack and map
	st := []rune{}

	// loop backwards over a string
	for idx := 0; idx < len(s); idx++ {
		r := rune(s[idx])
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

const DIGITS = "0123456789"

func findParentKeyIdx(der string, currentIdx int) int {
	// Go back until we find an unmatched open bracket
	bracketIdx := findMismatchedBracketRight(der[:currentIdx])
	if bracketIdx < 0 {
		return -1
	}
	for i := bracketIdx - 2; i >= 0; i-- {
		if !strings.Contains(DIGITS, string(der[i])) {
			return i + 1
		}
	}
	return -1
}

func (p *ProtoscopeDoc) String() string {
	return p.text
}

func corruptKeyAt(der string, startIdx int) string {
	key := ""
	endIdx := -1
	i := startIdx
	for ; i < len(der); i++ {
		if strings.Contains(DIGITS, string(der[i])) {
			key += string(der[i])
		} else {
			endIdx = i
			break
		}
	}
	if key == "" {
		log.Panicf("Could not find key, searched in [%d:%d], der[%d] = %s", startIdx, i, i, string(der[i]))
	}
	corruptedKey := "-1"
	if len(key) <= 3 {
		// This is an index key
		corruptedKey = "999"
	} else {
		corruptedKey = corruptKey(key)
	}
	der = der[:startIdx] + corruptedKey + der[endIdx:]
	return der
}

func (p *ProtoscopeDoc) Corrupt(rule *ProtoCorruptKeyRule) int {
	indices := allIndicesOf(p.text, rule.key+":")
	if len(indices) == 0 {
		return 0
	}
	modCount := 0
	for _, startIdx := range indices {
		endIdx := findFieldEnd(p.text[startIdx:])
		if rule.predicate(p.text[startIdx : startIdx+endIdx]) {
			if rule.corrupt != nil {
				p.text = rule.corrupt(p.text, startIdx, endIdx)
			} else {
				p.text = corruptKeyAt(p.text, startIdx)
			}
			modCount++
		}
	}
	return modCount
}

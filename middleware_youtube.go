package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/protocolbuffers/protoscope"
)

type YoutubeAdblockMiddleware struct{}

func NewYoutubeAdblockMiddleware() *YoutubeAdblockMiddleware {
	return &YoutubeAdblockMiddleware{}
}

func (mw *YoutubeAdblockMiddleware) Register(proxy *goproxy.ProxyHttpServer) {
	HOSTS_REGEXP := regexp.MustCompile(`\.youtube\.com|google\.(com|ca)|googleapis\.com|googleadservices\.com|googlevideo\.com`)

	proxy.OnRequest(goproxy.ReqHostMatches(HOSTS_REGEXP)).
		HandleConnect(goproxy.AlwaysMitm)

	proxy.OnResponse(goproxy.ReqHostMatches(HOSTS_REGEXP)).
		DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
			contentType := resp.Header.Get("Content-Type")

			if contentType == "application/x-protobuf" {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					log.Printf("Error reading response body")
					return resp
				}
				body, _ = removeAdsBytes(body)

				// Rewrite the response body
				resp.Body = io.NopCloser(bytes.NewReader(body))
				return resp
			} else {
				return resp
			}
		})
}

func deserializeProto(body []byte) string {
	return protoscope.Write(body, protoscope.WriterOptions{
		AllFieldsAreMessages:   false,
		ExplicitLengthPrefixes: false,
		NoGroups:               true,
		PrintFieldNames:        false,
	})
}

func serializeProto(protoTxt string) []byte {
	scanner := protoscope.NewScanner(protoTxt)
	outBytes, err := scanner.Exec()
	if err != nil {
		log.Printf("Error scanning - %v", err)
		return nil
	}
	return outBytes
}

func RemoveAds(body []byte) []byte {
	//return removeAdsBytes(body)
	return removeAdsProtoscope(body)
}

func removeAdsProtoscope(body []byte) []byte {
	if !bytes.Contains(body, []byte("/pagead/")) {
		return body
	}

	protoTxt := deserializeProto(body)

	protoDoc := NewProtoscopeDoc(protoTxt)
	corruptRules := []*ProtoCorruptKeyRule{
		NewProtoCorruptKeyRule("172660663", FieldValueContains("fullscreen_engagement_companion.eml")),
		NewProtoCorruptKeyRule("62960614", FieldValueContains("https://www.googleadservices.com")),
		NewProtoCorruptKeyRule("62960614", FieldValueContains("Visit advertiser")),
	}
	for _, corruptRule := range corruptRules {
		protoDoc.Corrupt(corruptRule)
	}

	// Find the field key
	return serializeProto(protoTxt)
}

func removeAdsBytes(body []byte) ([]byte, int) {
	const SEARCH_DISTANCE_LIMIT = 900
	AD_URL_SEARCH_STRING := []byte(".com/pagead/")
	targetFieldKeys := []*FieldKey{
		NewFieldKey(62960614, "Video ad"),
		NewFieldKey(378585263, "Page ad?"),
	}

	// Find all indices of the ad URL in body
	indices := []int{}
	searchStart := 0
	for {
		urlIdx := bytes.Index(body[searchStart:], AD_URL_SEARCH_STRING)
		if urlIdx < 0 {
			break
		}
		//log.Printf("Found ad URL at %d", urlIdx+searchStart)
		indices = append(indices, urlIdx+searchStart)
		searchStart = urlIdx + searchStart + 1
		//log.Printf("Search start %d", searchStart)
	}
	if len(indices) == 0 {
		// No ad URLs found, no modifications
		return body, 0
	}

	log.Printf("Ads detected")
	log.Printf("Found %d ad URLs", len(indices))
	// Dump the body to a file in data for debugging
	os.WriteFile("data/"+fmt.Sprint(time.Now().Unix())+"-raw.bin", body, 06440)

	// Modify the body
	modsCount := 0
	for _, urlIdx := range indices {
		start := urlIdx - SEARCH_DISTANCE_LIMIT
		if start < 0 {
			start = 0
		}
		searchSlice := body[start:urlIdx]
		for _, targetFieldKey := range targetFieldKeys {
			// Find the field key in searchSlice
			fieldKeyIdx := bytes.Index(searchSlice, targetFieldKey.Tag)
			// If found then replace it with the corrupted tag
			if fieldKeyIdx >= 0 {
				modsCount = modsCount + 1
				//log.Printf("Found field key %d at %d", targetFieldKey.Key, fieldKeyIdx+start)
				copy(searchSlice[fieldKeyIdx:], targetFieldKey.CorruptedTag)
			}
		}
	}
	if modsCount == 0 {
		log.Printf("No field keys found")
	} else {
		log.Printf("Body modified: %d field keys replaced", modsCount)
		os.WriteFile("data/"+fmt.Sprint(time.Now().Unix())+"-processed.bin", body, 06440)
	}
	return body, modsCount
}

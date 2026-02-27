// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

// Package utils provides helper functions for templ UI components.
// Based on github.com/templui/templui/internal/utils (MIT License).
package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	twmerge "github.com/Oudwins/tailwind-merge-go"
	"github.com/a-h/templ"
)

// ScriptVersion is a build-time version string used for cache-busting JS files.
// It is set at startup via SetScriptVersion.
var scriptVersion = "1"

// SetScriptVersion configures the cache-busting version string (call once at startup).
func SetScriptVersion(v string) {
	if v != "" {
		scriptVersion = v
	}
}

// ScriptURL returns the path with a cache-busting version query parameter.
func ScriptURL(path string) string {
	return fmt.Sprintf("%s?v=%s", path, scriptVersion)
}

// TwMerge combines Tailwind CSS classes and resolves conflicts using tailwind-merge.
// Example: TwMerge("bg-red-500", "bg-green-500") → "bg-green-500"
func TwMerge(classes ...string) string {
	return twmerge.Merge(classes...)
}

// If returns value if condition is true, otherwise the zero value of T.
// Useful for conditional Tailwind class inclusion.
func If[T comparable](condition bool, value T) T {
	var empty T
	if condition {
		return value
	}
	return empty
}

// IfElse returns trueValue if condition is true, otherwise falseValue.
func IfElse[T any](condition bool, trueValue T, falseValue T) T {
	if condition {
		return trueValue
	}
	return falseValue
}

// MergeAttributes combines multiple templ.Attributes maps into a single map.
// Later maps override earlier ones for the same key.
func MergeAttributes(attrs ...templ.Attributes) templ.Attributes {
	merged := make(templ.Attributes)
	for _, a := range attrs {
		for k, v := range a {
			merged[k] = v
		}
	}
	return merged
}

// RandomID returns a short random hex string suitable for HTML element IDs.
func RandomID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "id-fallback"
	}
	return "tui-" + hex.EncodeToString(b)
}

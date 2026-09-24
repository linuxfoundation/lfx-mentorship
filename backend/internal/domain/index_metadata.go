// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"context"
	"strings"
)

type indexMetadataKey struct{}

func ContextWithIndexHeaders(ctx context.Context, headers map[string]string) context.Context {
	return context.WithValue(ctx, indexMetadataKey{}, headers)
}

func IndexHeadersFromContext(ctx context.Context) map[string]string {
	headers, _ := ctx.Value(indexMetadataKey{}).(map[string]string)
	return headers
}

// SanitizedIndexHeaders removes secrets while preserving caller context needed by index consumers.
func SanitizedIndexHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		lower := strings.ToLower(strings.TrimSpace(key))
		switch lower {
		case "authorization":
			if strings.TrimSpace(value) != "" {
				out[lower] = "present"
			}
		default:
			out[lower] = value
		}
	}
	return out
}
